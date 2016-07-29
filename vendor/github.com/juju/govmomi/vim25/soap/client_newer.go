// +build go1.3

package soap

import (
	"net/http"
	"time"
)

func setTLSHandshakeTimeout(t *http.Transport, d time.Duration) {
	t.TLSHandshakeTimeout = d
}
