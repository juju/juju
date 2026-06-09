// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	"github.com/juju/juju/internal/wrench"
)

const (
	logSinkWrenchCategory   = "logsink"
	logSink503WrenchFeature = "return-503"
)

var logSink503WrenchActive = func() bool {
	return wrench.IsActive(logSinkWrenchCategory, logSink503WrenchFeature)
}

func maybeWrapLogSink503Wrench(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if logSink503WrenchActive() {
			logger.Warningf(req.Context(), "logsink QA wrench returning HTTP 503")
			http.Error(w, "logsink unavailable", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, req)
	})
}
