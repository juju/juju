// +build go1.2,!go1.3

package soap

import (
	"net/http"
	"time"
)

func setTLSHandshakeTimeout(t *http.Transport, d time.Duration) {
}
