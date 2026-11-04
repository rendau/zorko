package ingest

import (
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/andybalholm/brotli"
)

var (
	errBodyTooLarge        = errors.New("body too large")
	errUnsupportedEncoding = errors.New("unsupported content-encoding")
)

// readBody читает тело с распаковкой; предел max действует и на сжатое, и на распакованное тело,
// так что «сжатая бомба» обрывается на max байтах, а не распаковывается в память целиком
func readBody(r *http.Request, max int64) ([]byte, error) {
	if r.ContentLength > max {
		return nil, errBodyTooLarge
	}

	raw := &limitedReader{r: r.Body, left: max}

	var reader io.Reader
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))) {
	case "", "identity":
		reader = raw
	case "gzip", "x-gzip":
		gz, err := gzip.NewReader(raw)
		if err != nil {
			return nil, decodeErr(err)
		}
		defer gz.Close()
		reader = gz
	case "deflate":
		zr, err := zlib.NewReader(raw)
		if err != nil {
			return nil, decodeErr(err)
		}
		defer zr.Close()
		reader = zr
	case "br":
		reader = brotli.NewReader(raw)
	default:
		return nil, errUnsupportedEncoding
	}

	body, err := io.ReadAll(&limitedReader{r: reader, left: max})
	if err != nil {
		return nil, decodeErr(err)
	}

	return body, nil
}

func decodeErr(err error) error {
	if errors.Is(err, errBodyTooLarge) {
		return errBodyTooLarge
	}
	return fmt.Errorf("decode body: %w", err)
}

// limitedReader — как io.LimitReader, но превышение предела — ошибка, а не тихий обрыв
type limitedReader struct {
	r    io.Reader
	left int64
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.left < 0 {
		return 0, errBodyTooLarge
	}
	if int64(len(p)) > l.left+1 {
		p = p[:l.left+1]
	}
	n, err := l.r.Read(p)
	l.left -= int64(n)
	if l.left < 0 {
		return n, errBodyTooLarge
	}
	return n, err
}
