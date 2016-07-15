// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package httpstest

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CreateResponse creates test response from HTTP code
func CreateResponse(code int) (*http.Response, error) {
	dateTime := time.Now().UTC().Format(time.RFC1123)
	msg := fmt.Sprintf(`HTTP/1.1 %d %s
Server: cloudflare-nginx
Date: %s
Transfer-Encoding: chunked
Connection: keep-alive
Set-Cookie: __cfduid=d1cccd774afa317fcc34389711346edb61397809184442; expires=Mon, 23-Dec-2019 23:50:00 GMT; path=/; domain=.cloudsigma.com; HttpOnly
X-API-Version: Neon.prod.06e860ef2cb5+
CF-RAY: 11cf706acb32088d-FRA

`, code, http.StatusText(code), dateTime)
	return CreateResponseFromString(msg)
}

// CreateResponseWithType creates test response from HTTP code and content type
func CreateResponseWithType(code int, contentType string) (*http.Response, error) {
	dateTime := time.Now().UTC().Format(time.RFC1123)
	msg := fmt.Sprintf(`HTTP/1.1 %d %s
Server: cloudflare-nginx
Date: %s
Content-Type: %s
Transfer-Encoding: chunked
Connection: keep-alive
Set-Cookie: __cfduid=d1cccd774afa317fcc34389711346edb61397809184442; expires=Mon, 23-Dec-2019 23:50:00 GMT; path=/; domain=.cloudsigma.com; HttpOnly
X-API-Version: Neon.prod.06e860ef2cb5+
CF-RAY: 11cf706acb32088d-FRA

`, code, http.StatusText(code), dateTime, contentType)
	return CreateResponseFromString(msg)
}

// CreateResponseWithBody creates test response from HTTP code, content type and body content
func CreateResponseWithBody(code int, contentType string, data string) (*http.Response, error) {
	dateTime := time.Now().UTC().Format(time.RFC1123)
	ds := fmt.Sprintf("%x\r\n%s\r\n0\r\n\r\n", len(data), data)
	msg := fmt.Sprintf(`HTTP/1.1 %d %s
Server: cloudflare-nginx
Date: %s
Content-Type: %s
Transfer-Encoding: chunked
Connection: keep-alive
Set-Cookie: __cfduid=d1cccd774afa317fcc34389711346edb61397809184442; expires=Mon, 23-Dec-2019 23:50:00 GMT; path=/; domain=.cloudsigma.com; HttpOnly
X-API-Version: Neon.prod.06e860ef2cb5+
CF-RAY: 11cf706acb32088d-FRA

%s`, code, http.StatusText(code), dateTime, contentType, ds)
	return CreateResponseFromString(msg)
}

// CreateResponseFromString creates test response from string
func CreateResponseFromString(s string) (*http.Response, error) {
	r := bufio.NewReader(strings.NewReader(s))
	return http.ReadResponse(r, nil)
}
