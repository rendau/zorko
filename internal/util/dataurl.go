package util

import (
	"encoding/base64"
	"net/url"
	"strings"
)

// DecodeDataUrl — содержимое data:-адреса (base64 или процентная кодировка); не data: или битый — nil
func DecodeDataUrl(ref string) []byte {
	rest, ok := strings.CutPrefix(ref, "data:")
	if !ok {
		return nil
	}
	meta, data, ok := strings.Cut(rest, ",")
	if !ok {
		return nil
	}
	if strings.HasSuffix(meta, ";base64") {
		b, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			b, err = base64.RawStdEncoding.DecodeString(data)
		}
		if err != nil {
			return nil
		}
		return b
	}
	s, err := url.PathUnescape(data)
	if err != nil {
		return nil
	}
	return []byte(s)
}
