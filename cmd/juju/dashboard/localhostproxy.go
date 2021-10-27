// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard

import (
	"net/http"
	"net/http/httputil"
)

type dashboardProxy struct {
	dashboardURL string
}

func (p *dashboardProxy) transparentHttpProxy() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		director := func(target *http.Request) {
			target.URL.Scheme = "http"
			target.URL.Path = r.URL.Path
			target.URL.Host = p.dashboardURL
		}
		proxy := &httputil.ReverseProxy{Director: director}
		proxy.ServeHTTP(w, r)
	}
}
