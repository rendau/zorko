//go:build integration

package sentryapi_test

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1" //nolint:gosec
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainArtifactRepoBlobP "github.com/rendau/zorko/internal/domain/artifact/repo/blob"
	domainArtifactRepoDbP "github.com/rendau/zorko/internal/domain/artifact/repo/db"
	domainArtifactServiceP "github.com/rendau/zorko/internal/domain/artifact/service"
	domainOrgRepoDbP "github.com/rendau/zorko/internal/domain/org/repo/db"
	domainOrgServiceP "github.com/rendau/zorko/internal/domain/org/service"
	domainOrgTokenRepoDbP "github.com/rendau/zorko/internal/domain/org_token/repo/db"
	domainOrgTokenServiceP "github.com/rendau/zorko/internal/domain/org_token/service"
	domainProjectRepoDbP "github.com/rendau/zorko/internal/domain/project/repo/db"
	domainProjectServiceP "github.com/rendau/zorko/internal/domain/project/service"
	domainReleaseRepoDbP "github.com/rendau/zorko/internal/domain/release/repo/db"
	domainReleaseServiceP "github.com/rendau/zorko/internal/domain/release/service"
	"github.com/rendau/zorko/internal/handler/sentryapi"
	serviceAssembleServiceP "github.com/rendau/zorko/internal/service/assemble/service"
	"github.com/rendau/zorko/internal/testutil"
	"github.com/rendau/zorko/internal/usecase/upload"
)

type env struct {
	srv       *httptest.Server
	token     string
	orgSlug   string
	projSlug  string
	projectId int64
	orgId     int64
	t         *testing.T
	pool      *pgxpool.Pool
}

func setup(t *testing.T) *env {
	ctx := context.Background()
	pool := testutil.PgPool(t)
	projectId := testutil.NewProject(t, pool)
	e := &env{t: t, projectId: projectId, projSlug: "p", pool: pool}
	require.NoError(t, pool.QueryRow(ctx, `select o.id, o.slug from project p join org o on o.id = p.org_id where p.id = $1`, projectId).
		Scan(&e.orgId, &e.orgSlug))

	tokenSvc := domainOrgTokenServiceP.New(domainOrgTokenRepoDbP.New(pool))
	created, err := tokenSvc.Create(ctx, e.orgId, e.orgSlug, nil, "ci")
	require.NoError(t, err)
	e.token = created.Token

	artifactSvc := domainArtifactServiceP.New(domainArtifactRepoDbP.New(pool), domainArtifactRepoBlobP.New(testutil.S3(t)))
	assembler := serviceAssembleServiceP.New(serviceAssembleServiceP.Config{TmpDir: t.TempDir()}, artifactSvc)
	assembler.Start(ctx)
	t.Cleanup(assembler.Wait)

	uc := upload.New(true, tokenSvc, domainOrgServiceP.New(domainOrgRepoDbP.New(pool)),
		domainProjectServiceP.New(domainProjectRepoDbP.New(pool)), artifactSvc, assembler,
		domainReleaseServiceP.New(domainReleaseRepoDbP.New(pool)))
	e.srv = httptest.NewServer(sentryapi.New(uc).Router())
	t.Cleanup(e.srv.Close)
	return e
}

func (e *env) do(method, path, token string, body io.Reader, contentType string) (int, map[string]any) {
	req, err := http.NewRequest(method, e.srv.URL+path, body)
	require.NoError(e.t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:forbidigo // тестовый сервер
	require.NoError(e.t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func (e *env) json(method, path string, v any) (int, map[string]any) {
	b, _ := json.Marshal(v)
	return e.do(method, path, e.token, bytes.NewReader(b), "application/json")
}

func sha1hex(b []byte) string {
	s := sha1.Sum(b) //nolint:gosec
	return hex.EncodeToString(s[:])
}

func buildBundle(t *testing.T, debugId string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, content string) {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, _ = w.Write([]byte(content))
	}
	write("files/_/_/index.js", "function a(){throw new Error('x')}\n//# debugId="+debugId+"\n//# sourceMappingURL=index.js.map")
	write("files/_/_/index.js.map", `{"version":3,"sources":["src/a.js"],"names":[],"mappings":"AAAA"}`)
	mf := map[string]any{
		"org": "o", "release": "shop@1.0.0",
		"files": map[string]any{
			"files/_/_/index.js":     map[string]any{"url": "~/assets/index.js", "type": "minified_source", "headers": map[string]string{"debug-id": debugId, "Sourcemap": "index.js.map"}},
			"files/_/_/index.js.map": map[string]any{"url": "~/assets/index.js.map", "type": "source_map", "headers": map[string]string{"debug-id": debugId}},
		},
	}
	mfb, _ := json.Marshal(mf)
	write("manifest.json", string(mfb))
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func (e *env) uploadChunks(chunks [][]byte, gz bool) int {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, c := range chunks {
		field := "file"
		data := c
		if gz {
			field = "file_gzip"
			var zb bytes.Buffer
			zw := gzip.NewWriter(&zb)
			_, _ = zw.Write(c)
			_ = zw.Close()
			data = zb.Bytes()
		}
		w, err := mw.CreateFormFile(field, sha1hex(c))
		require.NoError(e.t, err)
		_, _ = w.Write(data)
	}
	require.NoError(e.t, mw.Close())
	status, _ := e.do(http.MethodPost, "/api/0/organizations/"+e.orgSlug+"/chunk-upload/", e.token, &body, mw.FormDataContentType())
	return status
}

func TestUploadFlow(t *testing.T) {
	e := setup(t)
	base := "/api/0/organizations/" + e.orgSlug

	// без токена, с чужим, с другой организацией
	status, out := e.do(http.MethodGet, base+"/chunk-upload/", "", nil, "")
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Equal(t, "Invalid token", out["detail"])
	status, _ = e.do(http.MethodGet, "/api/0/organizations/other-org/chunk-upload/", e.token, nil, "")
	assert.Equal(t, http.StatusForbidden, status)

	status, opts := e.do(http.MethodGet, base+"/chunk-upload/", e.token, nil, "")
	require.Equal(t, http.StatusOK, status)
	assert.EqualValues(t, 8<<20, opts["chunkSize"])
	assert.Equal(t, "sha1", opts["hashAlgorithm"])
	assert.Contains(t, opts["accept"], "artifact_bundles")

	debugId := uuid.NewString()
	bundle := buildBundle(t, debugId)
	// два куска: проверяет склейку по порядку
	half := len(bundle) / 2
	chunks := [][]byte{bundle[:half], bundle[half:]}
	checksum := sha1hex(bundle)
	assembleBody := map[string]any{"checksum": checksum, "chunks": []string{sha1hex(chunks[0]), sha1hex(chunks[1])}, "projects": []string{e.projSlug}}

	status, out = e.json(http.MethodPost, base+"/artifactbundle/assemble/", assembleBody)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "not_found", out["state"])
	assert.Len(t, out["missingChunks"], 2)

	assert.Equal(t, http.StatusOK, e.uploadChunks(chunks[:1], false))
	assert.Equal(t, http.StatusOK, e.uploadChunks(chunks[1:], true))

	// неверный отпечаток куска
	var bad bytes.Buffer
	mw := multipart.NewWriter(&bad)
	w, _ := mw.CreateFormFile("file", sha1hex([]byte("other")))
	_, _ = w.Write([]byte("content"))
	_ = mw.Close()
	status, _ = e.do(http.MethodPost, base+"/chunk-upload/", e.token, &bad, mw.FormDataContentType())
	assert.Equal(t, http.StatusBadRequest, status)

	state := ""
	for i := 0; i < 50 && state != "ok"; i++ {
		status, out = e.json(http.MethodPost, base+"/artifactbundle/assemble/", assembleBody)
		require.Equal(t, http.StatusOK, status)
		state, _ = out["state"].(string)
		require.NotEqual(t, "error", state, fmt.Sprint(out["detail"]))
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, "ok", state)

	var files, withDebugId int
	var release string
	require.NoError(t, e.pool.QueryRow(context.Background(), `select count(*), count(f.debug_id), min(b.release)
		from artifact_file f join artifact_bundle b on b.id = f.bundle_id
		where b.org_id = $1 and b.checksum = $2`, e.orgId, checksum).Scan(&files, &withDebugId, &release))
	assert.Equal(t, 2, files)
	assert.Equal(t, 2, withDebugId)
	assert.Equal(t, "shop@1.0.0", release, "релиз из manifest.json, если не передан в запросе")
	var chunksLeft int
	for _, c := range assembleBody["chunks"].([]string) {
		ok, err := testutil.S3(t).Exists(context.Background(), "chunk/"+c)
		require.NoError(t, err)
		if ok {
			chunksLeft++
		}
	}
	assert.Zero(t, chunksLeft, "куски удаляются после сборки")

	// повтор того же архива — сразу ok
	status, out = e.json(http.MethodPost, base+"/artifactbundle/assemble/", assembleBody)
	assert.Equal(t, "ok", out["state"])

	// проект — числом (id), как шлёт sentry-cli
	status, out = e.json(http.MethodPost, base+"/artifactbundle/assemble/", map[string]any{
		"checksum": checksum, "chunks": assembleBody["chunks"], "projects": []int64{e.projectId}})
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "ok", out["state"])

	// неизвестный проект
	status, _ = e.json(http.MethodPost, base+"/artifactbundle/assemble/", map[string]any{
		"checksum": checksum, "chunks": assembleBody["chunks"], "projects": []string{"nope"}})
	assert.Equal(t, http.StatusNotFound, status)
}

func TestAssembleRejectsBadArchive(t *testing.T) {
	e := setup(t)
	base := "/api/0/organizations/" + e.orgSlug
	notZip := []byte("this is not a zip " + uuid.NewString())
	require.Equal(t, http.StatusOK, e.uploadChunks([][]byte{notZip}, false))
	body := map[string]any{"checksum": sha1hex(notZip), "chunks": []string{sha1hex(notZip)}, "projects": []string{e.projSlug}}

	state, detail := "", ""
	for i := 0; i < 50 && state != "error"; i++ {
		_, out := e.json(http.MethodPost, base+"/artifactbundle/assemble/", body)
		state, _ = out["state"].(string)
		detail, _ = out["detail"].(string)
		time.Sleep(100 * time.Millisecond)
	}
	assert.Equal(t, "error", state)
	assert.Contains(t, detail, "not a zip")

	// отпечаток архива не сходится с кусками
	other := []byte("x" + uuid.NewString())
	require.Equal(t, http.StatusOK, e.uploadChunks([][]byte{other}, false))
	body = map[string]any{"checksum": sha1hex([]byte("y")), "chunks": []string{sha1hex(other)}, "projects": []string{e.projSlug}}
	for i := 0; i < 50 && state != "error"; i++ {
		_, out := e.json(http.MethodPost, base+"/artifactbundle/assemble/", body)
		state, _ = out["state"].(string)
		detail, _ = out["detail"].(string)
		time.Sleep(100 * time.Millisecond)
	}
	state = ""
	for i := 0; i < 50 && state != "error"; i++ {
		_, out := e.json(http.MethodPost, base+"/artifactbundle/assemble/", body)
		state, _ = out["state"].(string)
		detail, _ = out["detail"].(string)
		time.Sleep(100 * time.Millisecond)
	}
	assert.Equal(t, "error", state)
	assert.Contains(t, detail, "checksum mismatch")
}

func TestReleases(t *testing.T) {
	e := setup(t)
	base := "/api/0/organizations/" + e.orgSlug
	version := "shop@" + uuid.NewString()[:8]

	status, out := e.json(http.MethodPost, base+"/releases/", map[string]any{"version": version, "projects": []string{e.projSlug}})
	require.Equal(t, http.StatusCreated, status)
	assert.Equal(t, version, out["version"])
	assert.NotEmpty(t, out["dateCreated"])
	require.Len(t, out["projects"], 1)

	status, _ = e.json(http.MethodPost, base+"/releases/", map[string]any{"version": version, "projects": []string{e.projSlug}})
	assert.Equal(t, http.StatusAlreadyReported, status)

	status, out = e.json(http.MethodPut, base+"/releases/"+version+"/", map[string]any{"dateReleased": time.Now().UTC(), "refs": []any{}})
	require.Equal(t, http.StatusOK, status)
	assert.NotNil(t, out["dateReleased"])

	status, _ = e.json(http.MethodPost, base+"/releases/"+version+"/deploys/", map[string]any{"environment": "production"})
	assert.Equal(t, http.StatusCreated, status)

	status, _ = e.json(http.MethodGet, base+"/releases/nope/", nil)
	assert.Equal(t, http.StatusNotFound, status)

	status, _ = e.json(http.MethodPost, base+"/releases/", map[string]any{"version": "a/b", "projects": []string{e.projSlug}})
	assert.Equal(t, http.StatusBadRequest, status)

	status, _ = e.json(http.MethodGet, base+"/unknown-thing/", nil)
	assert.Equal(t, http.StatusNotFound, status)
}
