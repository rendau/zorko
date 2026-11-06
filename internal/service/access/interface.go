package access

import (
	"context"

	"github.com/rendau/zorko/internal/service/access/model"
)

// Access — какие проекты видит вошедший человек (админ — все; участник — свои, пусто — все)
type Access interface {
	ProjectAccess(ctx context.Context) (*model.ProjectAccess, error)
	CheckProject(ctx context.Context, projectId int64) error
}
