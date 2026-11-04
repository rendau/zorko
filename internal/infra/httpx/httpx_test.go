package httpx

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewTransportDefaults(t *testing.T) {
	tr := NewTransport(Config{})
	assert.Equal(t, 2*time.Second, tr.TLSHandshakeTimeout)
	assert.Equal(t, 20, tr.MaxConnsPerHost)
	assert.Equal(t, 20, tr.MaxIdleConnsPerHost)
	assert.True(t, tr.TLSClientConfig.InsecureSkipVerify)

	tr = NewTransport(Config{MaxConnsPerHost: 4, TLSHandshakeTimeout: time.Second, NoProxy: true})
	assert.Equal(t, 4, tr.MaxConnsPerHost)
	assert.Equal(t, time.Second, tr.TLSHandshakeTimeout)
	assert.Nil(t, tr.Proxy)
}
