// loadgen — нагрузка на приём zorko событиями в формате Sentry (envelope) и замер скорости.
//
//	go run ./tools/loadgen -dsn http://<key>@localhost:8000/1 -n 100000 -c 64 -distinct 1
//
// -distinct — сколько разных ошибок (отпечатков) в потоке; события одной ошибки одинаковые,
// как при плохом релизе.
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
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	dsn := flag.String("dsn", "", "DSN ключа проекта: <схема>://<key>@<хост>/<project_id>")
	total := flag.Int("n", 10000, "сколько событий отправить")
	concurrency := flag.Int("c", 32, "сколько запросов одновременно")
	distinct := flag.Int("distinct", 1, "сколько разных ошибок в потоке")
	origin := flag.String("origin", "", "заголовок Origin (проверка разрешённых сайтов)")
	flag.Parse()

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
