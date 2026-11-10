package replayseg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendau/zorko/internal/constant"
	sessionService "github.com/rendau/zorko/internal/domain/session/service"
	"github.com/rendau/zorko/internal/errs"
	authnService "github.com/rendau/zorko/internal/service/authn/service"
)

type usecaseFake struct {
	sessions *sessionService.Service
	err      error
	got      struct {
		projectId int64
		replayId  string
		segmentId int
	}
}

func (f *usecaseFake) SegmentBody(ctx context.Context, projectId int64, replayId string, segmentId int) ([]byte, error) {
	if !f.sessions.CtxIsAuthorized(ctx) {
		return nil, errs.NotAuthorized
	}
	f.got.projectId, f.got.replayId, f.got.segmentId = projectId, replayId, segmentId
	if f.err != nil {
		return nil, f.err
	}
	return []byte{0x78, 0x9c, 1, 2}, nil
}

func TestSegment(t *testing.T) {
	sessions := sessionService.New("secret")
	token, err := sessions.CreateToken(2, false)
	require.NoError(t, err)
	authn, err := authnService.New(authnService.Config{Mode: constant.AuthModeLocal}, sessions, nil)
	require.NoError(t, err)

	do := func(uc *usecaseFake, auth string, n string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/replay/abc/segment/"+n+"?project_id=10", nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		New(uc, sessions, authn).Segment(w, r, map[string]string{"id": "abc", "n": n})
		return w
	}

	uc := &usecaseFake{sessions: sessions}
	assert.Equal(t, http.StatusUnauthorized, do(uc, "", "0").Code, "без входа")
	assert.Equal(t, http.StatusUnauthorized, do(uc, "Bearer broken", "0").Code)

	w := do(uc, "Bearer "+token, "3")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "private, max-age=86400", w.Header().Get("Cache-Control"))
	assert.Equal(t, []byte{0x78, 0x9c, 1, 2}, w.Body.Bytes())
	assert.EqualValues(t, 10, uc.got.projectId)
	assert.Equal(t, "abc", uc.got.replayId)
	assert.Equal(t, 3, uc.got.segmentId)

	assert.Equal(t, http.StatusBadRequest, do(uc, "Bearer "+token, "x").Code)

	for err, code := range map[error]int{
		errs.NoPermission:   http.StatusForbidden,
		errs.ReplayNotFound: http.StatusNotFound,
		errs.ServiceNA:      http.StatusServiceUnavailable,
	} {
		uc.err = err
		assert.Equal(t, code, do(uc, "Bearer "+token, "0").Code, err.Error())
	}
}
