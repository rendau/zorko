package upload

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/samber/lo"

	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	releaseModel "github.com/rendau/zorko/internal/domain/release/model"
	"github.com/rendau/zorko/internal/errs"
)

type ReleaseServiceI interface {
	Get(ctx context.Context, orgId int64, version string) (*releaseModel.Main, bool, error)
	Ensure(ctx context.Context, orgId int64, version string, projectIds []int64, dateReleased *time.Time) (*releaseModel.Main, bool, error)
	SetDateReleased(ctx context.Context, id int64, at time.Time) error
}

// ReleaseInfo — релиз для ответа в формате Sentry
type ReleaseInfo struct {
	Version      string
	DateCreated  time.Time
	DateReleased *time.Time
	Projects     []*projectModel.Main
}

const releaseVersionMaxLen = 250

// validVersion — правила Sentry: не пусто, без / \ и переводов строк, не «.», «..», « », «latest»
func validVersion(v string) error {
	if v == "" || utf8.RuneCountInString(v) > releaseVersionMaxLen || strings.ContainsAny(v, "/\\\n\r\t") ||
		v == "." || v == ".." || strings.TrimSpace(v) == "" || strings.EqualFold(v, "latest") {
		return errs.ErrFull{Err: errs.InvalidRequest, Desc: "invalid release version"}
	}
	return nil
}

func (u *Usecase) CreateRelease(ctx context.Context, auth *Auth, version string, projects []string, dateReleased *time.Time) (*ReleaseInfo, bool, error) {
	if err := validVersion(version); err != nil {
		return nil, false, err
	}
	projectIds, err := u.resolveProjects(ctx, auth.OrgId, projects)
	if err != nil {
		return nil, false, err
	}
	rel, created, err := u.releaseSvc.Ensure(ctx, auth.OrgId, version, projectIds, dateReleased)
	if err != nil {
		return nil, false, fmt.Errorf("releaseSvc.Ensure: %w", err)
	}
	info, err := u.releaseInfo(ctx, auth, rel)
	return info, created, err
}

func (u *Usecase) GetRelease(ctx context.Context, auth *Auth, version string) (*ReleaseInfo, error) {
	rel, found, err := u.releaseSvc.Get(ctx, auth.OrgId, version)
	if err != nil {
		return nil, fmt.Errorf("releaseSvc.Get: %w", err)
	}
	if !found {
		return nil, errs.ReleaseNotFound
	}
	return u.releaseInfo(ctx, auth, rel)
}

// UpdateRelease — отметка «выкачен» (dateReleased) и дописка проектов; коммиты и refs не хранятся (шаг 4)
func (u *Usecase) UpdateRelease(ctx context.Context, auth *Auth, version string, projects []string, dateReleased *time.Time) (*ReleaseInfo, error) {
	rel, found, err := u.releaseSvc.Get(ctx, auth.OrgId, version)
	if err != nil {
		return nil, fmt.Errorf("releaseSvc.Get: %w", err)
	}
	if !found {
		return nil, errs.ReleaseNotFound
	}
	var projectIds []int64
	if len(projects) > 0 {
		if projectIds, err = u.resolveProjects(ctx, auth.OrgId, projects); err != nil {
			return nil, err
		}
	}
	if len(projectIds) > 0 || dateReleased != nil {
		if rel, _, err = u.releaseSvc.Ensure(ctx, auth.OrgId, version, projectIds, dateReleased); err != nil {
			return nil, fmt.Errorf("releaseSvc.Ensure: %w", err)
		}
	}
	return u.releaseInfo(ctx, auth, rel)
}

func (u *Usecase) releaseInfo(ctx context.Context, auth *Auth, rel *releaseModel.Main) (*ReleaseInfo, error) {
	projects, _, err := u.projectSvc.List(ctx, &projectModel.ListReq{OrgId: &auth.OrgId})
	if err != nil {
		return nil, fmt.Errorf("projectSvc.List: %w", err)
	}
	return &ReleaseInfo{
		Version:      rel.Version,
		DateCreated:  rel.CreatedAt,
		DateReleased: rel.DateReleased,
		Projects: lo.Filter(projects, func(p *projectModel.Main, _ int) bool {
			return lo.Contains(rel.ProjectIds, p.Id)
		}),
	}, nil
}
