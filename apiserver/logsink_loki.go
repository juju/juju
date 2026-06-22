// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"net/http"

	"github.com/juju/juju/internal/services"
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

func maybeWrapLogSink503IfLokiEnabled(
	next http.Handler,
	controllerDomainServices services.ControllerDomainServices,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if lokiForwardingEnabled(req.Context(), controllerDomainServices) {
			logger.Warningf(req.Context(), "logsink returning HTTP 503: Loki forwarding enabled")
			http.Error(w, "logsink unavailable", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, req)
	})
}

var lokiForwardingEnabled = func(
	ctx context.Context,
	controllerDomainServices services.ControllerDomainServices,
) bool {
	enabled, err := controllerDomainServices.Logging().IsLokiEnabled(ctx)
	if err != nil {
		logger.Errorf(ctx, "checking Loki config: %v", err)
		return false
	}
	return enabled
}
