package project

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/samber/lo"

	"github.com/rendau/zorko/internal/domain/project/model"
	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/util"
)

type Usecase struct {
	svc        ServiceI
	sessionSvc SessionServiceI
	accessSvc  AccessServiceI
}

func New(svc ServiceI, sessionSvc SessionServiceI, accessSvc AccessServiceI) *Usecase {
	return &Usecase{svc: svc, sessionSvc: sessionSvc, accessSvc: accessSvc}
}

func (u *Usecase) validateEdit(obj *model.Edit, forCreate bool) error {
	if forCreate && (obj.OrgId == nil || *obj.OrgId == 0) {
		return errs.ErrFull{Err: errs.InvalidRequest, Desc: "org_id is required", Fields: map[string]string{"org_id": "required"}}
	}
	if err := util.ValidateSlug(obj.Slug, forCreate); err != nil {
		return err
	}
	return validateSiteFetch(obj)
}

var headerNameRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// validateSiteFetch — хосты без схемы и пути (в нижнем регистре), имя заголовка, интервал от минуты до недели
func validateSiteFetch(obj *model.Edit) error {
	if obj.SiteFetchHost != nil {
		hosts := make([]string, 0, len(*obj.SiteFetchHost))
		for _, h := range *obj.SiteFetchHost {
			h = strings.ToLower(strings.TrimSpace(h))
			if h == "" {
				continue
			}
			if !util.IsHostPort(h) {
				return errs.ErrFull{Err: errs.InvalidRequest, Desc: "site_fetch_host: host expected, got " + h, Fields: map[string]string{"site_fetch_host": "invalid"}}
			}
			hosts = append(hosts, h)
		}
		obj.SiteFetchHost = new(lo.Uniq(hosts))
	}
	if obj.SiteFetchHeaderName != nil {
		*obj.SiteFetchHeaderName = strings.TrimSpace(*obj.SiteFetchHeaderName)
		if *obj.SiteFetchHeaderName != "" && !headerNameRe.MatchString(*obj.SiteFetchHeaderName) {
			return errs.ErrFull{Err: errs.InvalidRequest, Desc: "site_fetch_header_name: letters, digits and dash", Fields: map[string]string{"site_fetch_header_name": "invalid"}}
		}
	}
	if obj.SiteFetchHeaderValue != nil && (len(*obj.SiteFetchHeaderValue) > 512 || strings.ContainsAny(*obj.SiteFetchHeaderValue, "\r\n")) {
		return errs.ErrFull{Err: errs.InvalidRequest, Desc: "site_fetch_header_value: up to 512 characters, one line", Fields: map[string]string{"site_fetch_header_value": "invalid"}}
	}
	if obj.SiteFetchIntervalMin != nil && (*obj.SiteFetchIntervalMin < 1 || *obj.SiteFetchIntervalMin > 7*24*60) {
		return errs.ErrFull{Err: errs.InvalidRequest, Desc: "site_fetch_interval_min: from 1 to 10080", Fields: map[string]string{"site_fetch_interval_min": "invalid"}}
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
	access, err := u.accessSvc.ProjectAccess(ctx)
	if err != nil {
		return nil, 0, err
	}
	if !access.All {
		// участник видит только свои проекты
		pars.IDs = new(lo.Ternary(pars.IDs == nil, access.Ids, access.Filter(lo.FromPtr(pars.IDs))))
		if len(*pars.IDs) == 0 {
			return []*model.Main{}, 0, nil
		}
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
	if err := u.accessSvc.CheckProject(ctx, id); err != nil {
		return nil, err
	}
	result, _, err := u.svc.Get(ctx, id, true)
	if err != nil {
		return nil, fmt.Errorf("svc.Get: %w", err)
	}
	return result, nil
}

func (u *Usecase) Create(ctx context.Context, obj *model.Edit) (int64, error) {
	if err := u.sessionSvc.CtxRequireAdmin(ctx); err != nil {
		return 0, err
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

func (u *Usecase) Update(ctx context.Context, id int64, obj *model.Edit) error {
	if id == 0 {
		return errs.IdRequired
	}
	if err := u.sessionSvc.CtxRequireAdmin(ctx); err != nil {
		return err
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
	if id == 0 {
		return errs.IdRequired
	}
	if err := u.sessionSvc.CtxRequireAdmin(ctx); err != nil {
		return err
	}
	if err := u.svc.Delete(ctx, id); err != nil {
		return fmt.Errorf("svc.Delete: %w", err)
	}
	return nil
}
