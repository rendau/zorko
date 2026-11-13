package s3_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendau/zorko/internal/infra/s3"
)

const fakePage = 1000 // столько ключей S3 отдаёт за один запрос листинга

// fakeS3 — листинг (ListObjectsV2) и удаление пачкой (DeleteObjects) в памяти: ровно то, чем
// пользуется DeletePrefix. Листинг видит уже удалённое, как настоящий S3.
type fakeS3 struct {
	mu          sync.Mutex
	keys        map[string]bool
	lists       int
	deleteSizes []int

	onList     func(n int) (status int, body string) // n — номер запроса листинга с 1; status 0 — обычный ответ
	denyDelete func(key string) bool
}

type fakeListRep struct {
	XMLName               xml.Name         `xml:"ListBucketResult"`
	IsTruncated           bool             `xml:"IsTruncated"`
	NextContinuationToken string           `xml:"NextContinuationToken,omitempty"`
	Contents              []fakeContentRep `xml:"Contents"`
}

type fakeContentRep struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	Size         int    `xml:"Size"`
}

type fakeDeleteReq struct {
	Objects []struct {
		Key string `xml:"Key"`
	} `xml:"Object"`
}

type fakeDeleteRep struct {
	XMLName xml.Name `xml:"DeleteResult"`
	Deleted []struct {
		Key string `xml:"Key"`
	} `xml:"Deleted"`
	Errors []fakeDeleteErrRep `xml:"Error"`
}

type fakeDeleteErrRep struct {
	Key     string `xml:"Key"`
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

const fakeAccessDenied = `<Error><Code>AccessDenied</Code><Message>denied</Message></Error>`

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch {
	case r.Method == http.MethodGet && q.Has("list-type"):
		f.mu.Lock()
		f.lists++
		n := f.lists
		f.mu.Unlock()
		if f.onList != nil {
			if status, body := f.onList(n); status != 0 {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
				return
			}
		}

		f.mu.Lock()
		rest := make([]string, 0, len(f.keys))
		for k := range f.keys {
			if strings.HasPrefix(k, q.Get("prefix")) && k > q.Get("continuation-token") {
				rest = append(rest, k)
			}
		}
		f.mu.Unlock()
		slices.Sort(rest)

		rep := fakeListRep{}
		if len(rest) > fakePage {
			rest = rest[:fakePage]
			rep.IsTruncated, rep.NextContinuationToken = true, rest[len(rest)-1]
		}
		for _, k := range rest {
			rep.Contents = append(rep.Contents, fakeContentRep{Key: k, LastModified: "2026-11-01T00:00:00.000Z", Size: 1})
		}
		_ = xml.NewEncoder(w).Encode(rep)

	case r.Method == http.MethodPost && q.Has("delete"):
		req := fakeDeleteReq{}
		if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		rep := fakeDeleteRep{}
		f.mu.Lock()
		f.deleteSizes = append(f.deleteSizes, len(req.Objects))
		for _, o := range req.Objects {
			if f.denyDelete != nil && f.denyDelete(o.Key) {
				rep.Errors = append(rep.Errors, fakeDeleteErrRep{Key: o.Key, Code: "AccessDenied", Message: "denied"})
				continue
			}
			delete(f.keys, o.Key)
			rep.Deleted = append(rep.Deleted, struct {
				Key string `xml:"Key"`
			}{Key: o.Key})
		}
		f.mu.Unlock()
		_ = xml.NewEncoder(w).Encode(rep)

	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func (f *fakeS3) left(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for k := range f.keys {
		if strings.HasPrefix(k, prefix) {
			n++
		}
	}
	return n
}

// newFake — S3 в памяти с n объектами под day/ и тремя под other/ и клиент к нему
func newFake(t *testing.T, n int) (*fakeS3, *s3.Client) {
	t.Helper()
	f := &fakeS3{keys: map[string]bool{"other/1": true, "other/2": true, "other/3": true}}
	for i := range n {
		f.keys[fmt.Sprintf("day/%06d", i)] = true
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	c, err := s3.New(s3.Config{
		Endpoint: strings.TrimPrefix(srv.URL, "http://"), Region: "us-east-1", Bucket: "bucket", AccessKey: "a", SecretKey: "s",
	})
	require.NoError(t, err)
	return f, c
}

func TestDeletePrefix(t *testing.T) {
	f, c := newFake(t, 2500)

	n, err := c.DeletePrefix(context.Background(), "day/")
	require.NoError(t, err)
	assert.Equal(t, 2500, n)
	assert.Zero(t, f.left("day/"))
	assert.Equal(t, 3, f.left("other/"), "чужой префикс не тронут")

	// листинг идёт страницами, удаление — пачками не больше тысячи: весь список разом нигде не собирается
	assert.GreaterOrEqual(t, f.lists, 3)
	assert.Equal(t, 2500, sum(f.deleteSizes))
	for _, size := range f.deleteSizes {
		assert.LessOrEqual(t, size, 1000)
	}

	n, err = c.DeletePrefix(context.Background(), "day/")
	require.NoError(t, err)
	assert.Zero(t, n, "пустой префикс")
}

func sum(sizes []int) int {
	total := 0
	for _, s := range sizes {
		total += s
	}
	return total
}

func TestDeletePrefixListError(t *testing.T) {
	f, c := newFake(t, 2500)
	f.onList = func(n int) (int, string) {
		if n == 2 {
			return http.StatusForbidden, fakeAccessDenied
		}
		return 0, ""
	}

	n, err := c.DeletePrefix(context.Background(), "day/")
	require.ErrorContains(t, err, "ListObjects day/")
	assert.Equal(t, 1000, n, "первая страница удалена, счётчик — по факту")
	assert.Equal(t, 1500, f.left("day/"), "остальное — до следующего вызова")
}

func TestDeletePrefixRemoveError(t *testing.T) {
	f, c := newFake(t, 1200)
	f.denyDelete = func(key string) bool { return strings.HasSuffix(key, "7") }

	n, err := c.DeletePrefix(context.Background(), "day/")
	require.ErrorContains(t, err, "RemoveObjects day/")
	assert.Equal(t, 1080, n, "каждый десятый не удалился")
	assert.Equal(t, 120, f.left("day/"))
}

func TestDeletePrefixCanceled(t *testing.T) {
	f, c := newFake(t, 5000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// отмена посреди листинга: вызов обязан вернуться (листинг и удаление остановлены), а не зависнуть
	f.onList = func(n int) (int, string) {
		if n == 2 {
			cancel()
		}
		return 0, ""
	}

	n, err := c.DeletePrefix(ctx, "day/")
	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, n, 5000)
	assert.Positive(t, f.left("day/"))
	assert.Equal(t, 3, f.left("other/"))
}
