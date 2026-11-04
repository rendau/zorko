package usr

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	sessionService "github.com/rendau/zorko/internal/domain/session/service"
	"github.com/rendau/zorko/internal/domain/usr/model"
	usrService "github.com/rendau/zorko/internal/domain/usr/service"
	"github.com/rendau/zorko/internal/errs"
	loginlimitService "github.com/rendau/zorko/internal/service/loginlimit/service"
)

// repoFake — хранилище людей в памяти (domain service настоящий: хеширование и проверка пароля)
type repoFake struct {
	items  map[int64]*model.Main
	nextId int64
}

func (r *repoFake) List(context.Context, *model.ListReq) ([]*model.Main, int64, error) {
	result := make([]*model.Main, 0, len(r.items))
	for _, v := range r.items {
		result = append(result, v)
	}
	return result, int64(len(result)), nil
}

func (r *repoFake) Get(_ context.Context, id int64) (*model.Main, bool, error) {
	v, ok := r.items[id]
	if !ok {
		return nil, false, nil
	}
	c := *v
	return &c, true, nil
}

func (r *repoFake) GetByUsername(_ context.Context, username string) (*model.Main, bool, error) {
	for _, v := range r.items {
		if v.Username == username {
			c := *v
			return &c, true, nil
		}
	}
	return nil, false, nil
}

func (r *repoFake) HasAny(context.Context) (bool, error) { return len(r.items) > 0, nil }

func (r *repoFake) Create(_ context.Context, obj *model.Edit) (int64, error) {
	r.nextId++
	v := &model.Main{Id: r.nextId, Active: true}
	apply(v, obj)
	r.items[v.Id] = v
	return v.Id, nil
}

func (r *repoFake) Update(_ context.Context, id int64, obj *model.Edit) error {
	if v, ok := r.items[id]; ok {
		apply(v, obj)
	}
	return nil
}

func (r *repoFake) Delete(_ context.Context, id int64) error {
	delete(r.items, id)
	return nil
}

func apply(v *model.Main, obj *model.Edit) {
	if obj.Active != nil {
		v.Active = *obj.Active
	}
	if obj.IsAdmin != nil {
		v.IsAdmin = *obj.IsAdmin
	}
	if obj.Name != nil {
		v.Name = *obj.Name
	}
	if obj.Username != nil {
		v.Username = *obj.Username
	}
	if obj.Password != nil {
		v.Password = *obj.Password
	}
}

type env struct {
	uc       *Usecase
	sessions *sessionService.Service
}

func newEnv() *env {
	sessions := sessionService.New("test-secret")
	repo := &repoFake{items: map[int64]*model.Main{}}
	return &env{
		uc:       New(usrService.New(repo), sessions, loginlimitService.New(5, time.Minute)),
		sessions: sessions,
	}
}

func (e *env) as(id int64, admin bool) context.Context {
	return e.sessions.WithContext(context.Background(), &sessionModel.Session{Id: id, Admin: admin})
}

func TestBootstrapFirstAdmin(t *testing.T) {
	e := newEnv()
	ctx := context.Background()

	required, err := e.uc.BootstrapStatus(ctx)
	require.NoError(t, err)
	assert.True(t, required)

	id, err := e.uc.Create(ctx, &model.Edit{Username: new(" Admin "), Password: new("password-1"), IsAdmin: new(false)})
	require.NoError(t, err)

	admin, err := e.uc.Get(e.as(id, true), id)
	require.NoError(t, err)
	assert.True(t, admin.IsAdmin, "первый человек — всегда админ")
	assert.Equal(t, "admin", admin.Username, "логин приводится к нижнему регистру")

	required, _ = e.uc.BootstrapStatus(ctx)
	assert.False(t, required)

	_, err = e.uc.Create(ctx, &model.Edit{Username: new("intruder"), Password: new("password-1")})
	assert.ErrorIs(t, err, errs.NotAuthorized, "без входа второго не создать")
}

func TestLoginAndRefresh(t *testing.T) {
	e := newEnv()
	adminId, _ := e.uc.Create(context.Background(), &model.Edit{Username: new("admin"), Password: new("password-1")})
	adminCtx := e.as(adminId, true)
	olegId, err := e.uc.Create(adminCtx, &model.Edit{Username: new("oleg"), Password: new("temp-pass-1"), Active: new(true)})
	require.NoError(t, err)

	pair, err := e.uc.Login(context.Background(), "OLEG", "temp-pass-1", "10.0.0.1")
	require.NoError(t, err)
	session, err := e.sessions.FromToken(pair.Jwt)
	require.NoError(t, err)
	assert.Equal(t, olegId, session.Id)
	assert.False(t, session.IsAdmin())

	_, err = e.uc.Login(context.Background(), "oleg", "wrong", "10.0.0.1")
	assert.ErrorIs(t, err, errs.NotAuthorized)

	next, err := e.uc.RefreshToken(context.Background(), pair.RefreshToken)
	require.NoError(t, err)
	assert.NotEmpty(t, next.Jwt)

	// роль поменяли — новая пара несёт новую роль
	require.NoError(t, e.uc.Update(adminCtx, olegId, &model.Edit{IsAdmin: new(true)}))
	next, err = e.uc.RefreshToken(context.Background(), pair.RefreshToken)
	require.NoError(t, err)
	session, _ = e.sessions.FromToken(next.Jwt)
	assert.True(t, session.IsAdmin())

	// пароль сменили — старые refresh-токены отозваны
	require.NoError(t, e.uc.UpdateProfile(e.as(olegId, false), &UpdateProfileReq{Password: new("new-pass-1"), CurrentPassword: new("temp-pass-1")}))
	_, err = e.uc.RefreshToken(context.Background(), pair.RefreshToken)
	assert.ErrorIs(t, err, errs.NotAuthorized)

	// отключили — ни входа, ни обновления
	pair, _ = e.uc.Login(context.Background(), "oleg", "new-pass-1", "10.0.0.1")
	require.NoError(t, e.uc.Update(adminCtx, olegId, &model.Edit{Active: new(false)}))
	_, err = e.uc.RefreshToken(context.Background(), pair.RefreshToken)
	assert.ErrorIs(t, err, errs.NotAuthorized)
	_, err = e.uc.Login(context.Background(), "oleg", "new-pass-1", "10.0.0.2")
	assert.ErrorIs(t, err, errs.NotAuthorized)

	_, err = e.uc.RefreshToken(context.Background(), next.Jwt)
	assert.ErrorIs(t, err, errs.NotAuthorized, "access-токен — не refresh")
}

func TestLoginRateLimit(t *testing.T) {
	e := newEnv()
	_, _ = e.uc.Create(context.Background(), &model.Edit{Username: new("admin"), Password: new("password-1")})

	for range 5 {
		_, err := e.uc.Login(context.Background(), "admin", "wrong", "10.0.0.1")
		assert.ErrorIs(t, err, errs.NotAuthorized)
	}
	_, err := e.uc.Login(context.Background(), "admin", "password-1", "10.0.0.9")
	assert.ErrorIs(t, err, errs.LoginRateLimited, "логин заблокирован на минуту даже с верным паролем")

	_, err = e.uc.Login(context.Background(), "other", "x", "10.0.0.1")
	assert.ErrorIs(t, err, errs.LoginRateLimited, "и IP тоже")
}

func TestPermissions(t *testing.T) {
	e := newEnv()
	adminId, _ := e.uc.Create(context.Background(), &model.Edit{Username: new("admin"), Password: new("password-1")})
	adminCtx := e.as(adminId, true)
	memberId, _ := e.uc.Create(adminCtx, &model.Edit{Username: new("member"), Password: new("password-1")})
	memberCtx := e.as(memberId, false)

	_, err := e.uc.Create(memberCtx, &model.Edit{Username: new("x"), Password: new("password-1")})
	assert.ErrorIs(t, err, errs.NoPermission)
	assert.ErrorIs(t, e.uc.Update(memberCtx, adminId, &model.Edit{Name: new("x")}), errs.NoPermission)
	assert.ErrorIs(t, e.uc.Delete(memberCtx, adminId), errs.NoPermission)

	_, _, err = e.uc.List(context.Background(), &model.ListReq{})
	assert.ErrorIs(t, err, errs.NotAuthorized)

	assert.ErrorIs(t, e.uc.Update(adminCtx, adminId, &model.Edit{IsAdmin: new(false)}), errs.UsrSelfChange)
	assert.ErrorIs(t, e.uc.Update(adminCtx, adminId, &model.Edit{Active: new(false)}), errs.UsrSelfChange)
	assert.ErrorIs(t, e.uc.Delete(adminCtx, adminId), errs.UsrSelfChange)
	assert.NoError(t, e.uc.Delete(adminCtx, memberId))
}

func TestValidation(t *testing.T) {
	e := newEnv()
	cases := map[string]*model.Edit{
		"короткий пароль": {Username: new("admin"), Password: new("short")},
		"длинный пароль":  {Username: new("admin"), Password: new(string(make([]byte, 80)) + "x")},
		"без пароля":      {Username: new("admin")},
		"плохой логин":    {Username: new("a b"), Password: new("password-1")},
		"короткий логин":  {Username: new("ab"), Password: new("password-1")},
	}
	for name, obj := range cases {
		_, err := e.uc.Create(context.Background(), obj)
		full, ok := errs.AsErrFull(err)
		assert.True(t, ok && errors.Is(full.Err, errs.InvalidRequest), name)
	}

	adminId, _ := e.uc.Create(context.Background(), &model.Edit{Username: new("admin"), Password: new("password-1")})
	err := e.uc.UpdateProfile(e.as(adminId, true), &UpdateProfileReq{Password: new("new-pass-1"), CurrentPassword: new("wrong-current")})
	assert.Error(t, err, "смена пароля требует текущий")
}
