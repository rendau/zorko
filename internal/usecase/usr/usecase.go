package usr

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/rendau/zorko/internal/domain/usr/model"
	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/util"
)

const (
	passwordMinLen = 8  // в символах
	passwordMaxLen = 72 // в байтах: bcrypt молча обрезает длиннее
)

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)

type Usecase struct {
	svc          ServiceI
	sessionSvc   SessionServiceI
	loginLimiter LoginLimiterI
}

func New(svc ServiceI, sessionSvc SessionServiceI, loginLimiter LoginLimiterI) *Usecase {
	return &Usecase{svc: svc, sessionSvc: sessionSvc, loginLimiter: loginLimiter}
}

// Login — пара токенов. Не больше 5 неудачных попыток в минуту на логин и на IP.
func (u *Usecase) Login(ctx context.Context, username, password, clientIp string) (*TokenPair, error) {
	username = normalizeUsername(username)
	keys := []string{"usr:" + username, "ip:" + clientIp}

	for _, k := range keys {
		if !u.loginLimiter.Allowed(k) {
			return nil, errs.LoginRateLimited
		}
	}

	item, found, err := u.svc.AuthByUsernamePassword(ctx, username, password)
	if err != nil {
		return nil, fmt.Errorf("svc.AuthByUsernamePassword: %w", err)
	}
	if !found || !item.Active {
		for _, k := range keys {
			u.loginLimiter.Fail(k)
		}
		return nil, errs.NotAuthorized
	}

	return u.issueTokenPair(item)
}

// RefreshToken — новая пара по refresh-токену. Человек перечитывается из БД: отключение и смена
// роли действуют при ближайшем обновлении, смена пароля отзывает старые refresh-токены.
func (u *Usecase) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, errs.NotAuthorized
	}

	usrId, err := u.sessionSvc.RefreshTokenUserId(refreshToken)
	if err != nil {
		return nil, errs.NotAuthorized
	}

	item, found, err := u.svc.Get(ctx, usrId, false)
	if err != nil {
		return nil, fmt.Errorf("svc.Get: %w", err)
	}
	if !found || !item.Active {
		return nil, errs.NotAuthorized
	}

	if err = u.sessionSvc.CheckRefreshToken(refreshToken, item.Password); err != nil {
		return nil, errs.NotAuthorized
	}

	return u.issueTokenPair(item)
}

// BootstrapStatus — true, пока нет ни одного человека (можно создать первого админа без входа)
func (u *Usecase) BootstrapStatus(ctx context.Context) (bool, error) {
	hasAny, err := u.svc.HasAny(ctx)
	if err != nil {
		return false, fmt.Errorf("svc.HasAny: %w", err)
	}
	return !hasAny, nil
}

func (u *Usecase) GetProfile(ctx context.Context) (*model.Main, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, errs.NotAuthorized
	}
	item, _, err := u.svc.Get(ctx, u.sessionSvc.FromContext(ctx).Id, true)
	if err != nil {
		return nil, fmt.Errorf("svc.Get: %w", err)
	}
	return item, nil
}

func (u *Usecase) UpdateProfile(ctx context.Context, req *UpdateProfileReq) error {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return errs.NotAuthorized
	}
	id := u.sessionSvc.FromContext(ctx).Id

	obj := &model.Edit{Name: req.Name, Username: req.Username, Password: req.Password}
	if err := u.validateEdit(obj, false); err != nil {
		return err
	}

	if req.Password != nil {
		item, _, err := u.svc.Get(ctx, id, true)
		if err != nil {
			return fmt.Errorf("svc.Get: %w", err)
		}
		current := ""
		if req.CurrentPassword != nil {
			current = *req.CurrentPassword
		}
		if _, ok, err := u.svc.AuthByUsernamePassword(ctx, item.Username, current); err != nil {
			return fmt.Errorf("svc.AuthByUsernamePassword: %w", err)
		} else if !ok {
			return errs.ErrFull{Err: errs.InvalidRequest, Desc: "current password is wrong", Fields: map[string]string{"current_password": "invalid"}}
		}
	}

	if err := u.svc.Update(ctx, id, obj); err != nil {
		return fmt.Errorf("svc.Update: %w", err)
	}
	return nil
}

func (u *Usecase) List(ctx context.Context, pars *model.ListReq) ([]*model.Main, int64, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, 0, errs.NotAuthorized
	}
	if err := util.RequirePageSize(pars.ListParams, 0); err != nil {
		return nil, 0, err
	}
	items, tCount, err := u.svc.List(ctx, pars)
	if err != nil {
		return nil, 0, fmt.Errorf("svc.List: %w", err)
	}
	return items, tCount, nil
}

func (u *Usecase) Get(ctx context.Context, id int64) (*model.Main, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, errs.NotAuthorized
	}
	result, _, err := u.svc.Get(ctx, id, true)
	if err != nil {
		return nil, fmt.Errorf("svc.Get: %w", err)
	}
	return result, nil
}

// Create — админ; без входа — только первый человек, и он становится админом
func (u *Usecase) Create(ctx context.Context, obj *model.Edit) (int64, error) {
	session := u.sessionSvc.FromContext(ctx)
	if session.IsAuthorized() {
		if !session.IsAdmin() {
			return 0, errs.NoPermission
		}
	} else {
		hasAny, err := u.svc.HasAny(ctx)
		if err != nil {
			return 0, fmt.Errorf("svc.HasAny: %w", err)
		}
		if hasAny {
			return 0, errs.NotAuthorized
		}
		obj.IsAdmin = new(true)
		obj.Active = new(true)
	}

	if err := u.validateEdit(obj, true); err != nil {
		return 0, err
	}

	newId, err := u.svc.Create(ctx, obj)
	if err != nil {
		return 0, fmt.Errorf("svc.Create: %w", err)
	}
	return newId, nil
}

// Update — админ; себя нельзя отключить и лишить прав админа (иначе можно остаться без админов)
func (u *Usecase) Update(ctx context.Context, id int64, obj *model.Edit) error {
	if !u.sessionSvc.CtxIsAdmin(ctx) {
		return errs.NoPermission
	}
	if id == 0 {
		return errs.IdRequired
	}
	if id == u.sessionSvc.FromContext(ctx).Id &&
		((obj.Active != nil && !*obj.Active) || (obj.IsAdmin != nil && !*obj.IsAdmin)) {
		return errs.UsrSelfChange
	}
	if err := u.validateEdit(obj, false); err != nil {
		return err
	}
	if err := u.svc.Update(ctx, id, obj); err != nil {
		return fmt.Errorf("svc.Update: %w", err)
	}
	return nil
}

func (u *Usecase) Delete(ctx context.Context, id int64) error {
	if !u.sessionSvc.CtxIsAdmin(ctx) {
		return errs.NoPermission
	}
	if id == 0 {
		return errs.IdRequired
	}
	if id == u.sessionSvc.FromContext(ctx).Id {
		return errs.UsrSelfChange
	}
	if err := u.svc.Delete(ctx, id); err != nil {
		return fmt.Errorf("svc.Delete: %w", err)
	}
	return nil
}

func (u *Usecase) issueTokenPair(item *model.Main) (*TokenPair, error) {
	access, err := u.sessionSvc.CreateToken(item.Id, item.IsAdmin)
	if err != nil {
		return nil, fmt.Errorf("sessionSvc.CreateToken: %w", err)
	}
	refresh, err := u.sessionSvc.CreateRefreshToken(item.Id, item.Password)
	if err != nil {
		return nil, fmt.Errorf("sessionSvc.CreateRefreshToken: %w", err)
	}
	return &TokenPair{Jwt: access, RefreshToken: refresh}, nil
}

func (u *Usecase) validateEdit(obj *model.Edit, forCreate bool) error {
	if obj.Username != nil {
		obj.Username = new(normalizeUsername(*obj.Username))
	}
	if forCreate || obj.Username != nil {
		if obj.Username == nil || !usernameRe.MatchString(*obj.Username) {
			return errs.ErrFull{Err: errs.InvalidRequest, Desc: "username: a-z, 0-9, '.', '_', '-', 3–64 chars", Fields: map[string]string{"username": "invalid"}}
		}
	}
	if forCreate || obj.Password != nil {
		if obj.Password == nil {
			return errs.ErrFull{Err: errs.InvalidRequest, Desc: "password is required", Fields: map[string]string{"password": "required"}}
		}
		p := strings.TrimSpace(*obj.Password)
		if utf8.RuneCountInString(p) < passwordMinLen || len(p) > passwordMaxLen {
			return errs.ErrFull{Err: errs.InvalidRequest, Desc: "password: 8 chars minimum, 72 bytes maximum", Fields: map[string]string{"password": "invalid"}}
		}
	}
	return nil
}

func normalizeUsername(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
