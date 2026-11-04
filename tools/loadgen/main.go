// loadgen — нагрузка на приём zorko событиями в формате Sentry (envelope) и замер скорости.
//
//	go run ./tools/loadgen -dsn http://<key>@localhost:8000/1 -n 100000 -c 64 -distinct 1
//
// -distinct — сколько разных ошибок (отпечатков) в потоке; события одной ошибки одинаковые,
// как при плохом релизе.
//
// -js — сжатый файл сборки (после sentry-cli sourcemaps inject): кадры указывают на настоящие места в нём,
// событие несёт метку файла (debug_meta) — нагрузка на перевод стека по картам.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
)

// minified — кадры по настоящему сжатому файлу (режим -js)
var minified struct {
	url     string
	debugId string
	frames  string
}

func loadMinified(path string, needles []string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := string(b)
	i := strings.LastIndex(src, "//# debugId=")
	if i < 0 {
		return fmt.Errorf("no //# debugId= in %s (run sentry-cli sourcemaps inject)", path)
	}
	minified.debugId = strings.TrimSpace(strings.SplitN(src[i+len("//# debugId="):], "\n", 2)[0])
	minified.debugId = strings.Fields(minified.debugId)[0]
	minified.url = "https://shop.mechta.kz/assets/" + path[strings.LastIndex(path, "/")+1:]
	frames := make([]string, 0, len(needles))
	for _, n := range needles {
		off := strings.Index(src, n)
		if off < 0 {
			return fmt.Errorf("needle %q not found", n)
		}
		lineStart := strings.LastIndex(src[:off], "\n") + 1
		line := strings.Count(src[:off], "\n") + 1
		col := len(utf16.Encode([]rune(src[lineStart:off]))) + 1
		frames = append(frames, fmt.Sprintf(`{"filename":%q,"abs_path":%q,"function":"FN","lineno":%d,"colno":%d,"in_app":true}`,
			minified.url, minified.url, line, col))
	}
	// старые кадры первыми, место падения — последним
	slices.Reverse(frames)
	minified.frames = strings.Join(frames, ",")
	return nil
}

func main() {
	dsn := flag.String("dsn", "", "DSN ключа проекта: <схема>://<key>@<хост>/<project_id>")
	total := flag.Int("n", 10000, "сколько событий отправить")
	concurrency := flag.Int("c", 32, "сколько запросов одновременно")
	distinct := flag.Int("distinct", 1, "сколько разных ошибок в потоке")
	origin := flag.String("origin", "", "заголовок Origin (проверка разрешённых сайтов)")
	jsPath := flag.String("js", "", "сжатый JS с меткой //# debugId= — кадры по его местам и debug_meta")
	needles := flag.String("needles", "throw RangeError,t(this.rules.find,e.toFixed", "места в -js через запятую (старые кадры первыми)")
	flag.Parse()

	if *jsPath != "" {
		if err := loadMinified(*jsPath, strings.Split(*needles, ",")); err != nil {
			log.Fatalf("js: %v", err)
		}
	}

	endpoint, key, err := parseDsn(*dsn)
	if err != nil {
		log.Fatalf("dsn: %v", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
			TLSHandshakeTimeout: 2 * time.Second,
			MaxConnsPerHost:     *concurrency,
			MaxIdleConnsPerHost: *concurrency,
		},
	}

	var (
		next      atomic.Int64
		statusMu  sync.Mutex
		statuses  = map[int]int{}
		latencies = make([]time.Duration, *total)
		wg        sync.WaitGroup
	)

	start := time.Now()
	for range *concurrency {
		wg.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= *total {
					return
				}

				body := envelope(i % *distinct)
				req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/x-sentry-envelope")
				req.Header.Set("X-Sentry-Auth", "Sentry sentry_version=7, sentry_key="+key+", sentry_client=zorko-loadgen/1")
				if *origin != "" {
					req.Header.Set("Origin", *origin)
				}

				t := time.Now()
				resp, err := client.Do(req)
				latencies[i] = time.Since(t)

				code := -1
				if err == nil {
					code = resp.StatusCode
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}
				statusMu.Lock()
				statuses[code]++
				statusMu.Unlock()
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)

	slices.Sort(latencies)
	p := func(q float64) time.Duration { return latencies[min(int(float64(len(latencies))*q), len(latencies)-1)] }

	fmt.Printf("events: %d in %s → %.0f events/s\n", *total, elapsed.Round(time.Millisecond), float64(*total)/elapsed.Seconds())
	fmt.Printf("latency: p50 %s  p95 %s  p99 %s  max %s\n", p(0.50), p(0.95), p(0.99), latencies[len(latencies)-1])
	fmt.Printf("statuses: %v\n", statuses)
	if statuses[http.StatusOK] != *total {
		os.Exit(1)
	}
}

// parseDsn → адрес envelope и ключ
func parseDsn(dsn string) (string, string, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		return "", "", fmt.Errorf("bad dsn %q", dsn)
	}
	key := u.User.Username()
	projectId := u.Path[1:]
	if _, err = strconv.Atoi(projectId); err != nil {
		return "", "", fmt.Errorf("bad project id in dsn %q", dsn)
	}
	u.User, u.Path = nil, "/api/"+projectId+"/envelope/"
	return u.String(), key, nil
}

// envelope — событие браузерного SDK примерно в 2 КБ; variant меняет место падения (отпечаток)
func envelope(variant int) []byte {
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	eventId := hex.EncodeToString(id)
	ts := strconv.FormatFloat(float64(time.Now().UnixMilli())/1000, 'f', 3, 64)
	fn := "addItem" + strconv.Itoa(variant)

	if minified.url != "" {
		event := `{"event_id":"` + eventId + `","timestamp":` + ts + `,"platform":"javascript","level":"error",` +
			`"release":"shop@1.4.2","environment":"production",` +
			`"exception":{"values":[{"type":"RangeError","value":"Сумма корзины меньше 1000",` +
			`"mechanism":{"type":"onerror","handled":false},"stacktrace":{"frames":[` + strings.ReplaceAll(minified.frames, "FN", fn) + `]}}]},` +
			`"debug_meta":{"images":[{"type":"sourcemap","code_file":"` + minified.url + `","debug_id":"` + minified.debugId + `"}]},` +
			`"request":{"url":"https://shop.mechta.kz/cart","headers":{"User-Agent":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/130.0 Safari/537.36"}},` +
			`"user":{"id":"u-` + strconv.Itoa(int(id[0])%200) + `"},"sdk":{"name":"sentry.javascript.vue","version":"11.10.0"}}`
		return []byte(`{"event_id":"` + eventId + `"}` + "\n" + `{"type":"event"}` + "\n" + event + "\n")
	}

	event := `{"event_id":"` + eventId + `","timestamp":` + ts + `,"platform":"javascript","level":"error",` +
		`"release":"shop@1.4.2","environment":"production",` +
		`"exception":{"values":[{"type":"TypeError","value":"Cannot read properties of undefined (reading 'price')",` +
		`"mechanism":{"type":"onerror","handled":false},"stacktrace":{"frames":[` +
		`{"filename":"https://shop.mechta.kz/assets/vendor-Ab12Cd34.js","function":"callWithErrorHandling","lineno":1,"colno":4123,"in_app":false},` +
		`{"filename":"https://shop.mechta.kz/assets/vendor-Ab12Cd34.js","function":"invoker","lineno":1,"colno":9876,"in_app":false},` +
		`{"filename":"https://shop.mechta.kz/assets/CartPage-Xy98Zw76.js","function":"onClick","lineno":1,"colno":512,"in_app":true},` +
		`{"filename":"https://shop.mechta.kz/assets/CartPage-Xy98Zw76.js","function":"` + fn + `","lineno":1,"colno":733,"in_app":true}]}}]},` +
		`"request":{"url":"https://shop.mechta.kz/cart","headers":{"User-Agent":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/130.0 Safari/537.36"}},` +
		`"contexts":{"browser":{"name":"Chrome","version":"130.0.0"},"os":{"name":"macOS","version":"15.1"}},` +
		`"user":{"id":"u-` + strconv.Itoa(int(id[0])%200) + `"},"tags":{"page":"cart"},` +
		`"breadcrumbs":{"values":[{"timestamp":` + ts + `,"category":"ui.click","message":"button.add-to-cart"},` +
		`{"timestamp":` + ts + `,"category":"fetch","data":{"method":"GET","url":"/api/cart","status_code":200}},` +
		`{"timestamp":` + ts + `,"category":"navigation","data":{"from":"/catalog","to":"/cart"}}]},` +
		`"sdk":{"name":"sentry.javascript.vue","version":"10.0.0"}}`

	return []byte(`{"event_id":"` + eventId + `","sent_at":"` + time.Now().UTC().Format(time.RFC3339) + `"}` + "\n" +
		`{"type":"event"}` + "\n" + event + "\n")
}
