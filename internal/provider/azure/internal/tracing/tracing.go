// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracing

import (
	"net/http"
	"net/http/httputil"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"

	"github.com/juju/juju/core/logger"
)

type LoggingPolicy struct {
	Logger logger.Logger
}

func (p *LoggingPolicy) Do(req *policy.Request) (*http.Response, error) {
	if p.Logger.IsLevelEnabled(logger.TRACE) {
		dump, err := httputil.DumpRequest(req.Raw(), true)
		if err != nil {
			p.Logger.Tracef(req.Raw().Context(), "failed to dump request: %v", err)
			p.Logger.Tracef(req.Raw().Context(), "%+v", req.Raw())
		} else {
			p.Logger.Tracef(req.Raw().Context(), "%s", dump)
		}
	}
	resp, err := req.Next()
	if err == nil && p.Logger.IsLevelEnabled(logger.TRACE) {
		dump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			p.Logger.Tracef(req.Raw().Context(), "failed to dump response: %v", err)
			p.Logger.Tracef(req.Raw().Context(), "%+v", resp)
		} else {
			p.Logger.Tracef(req.Raw().Context(), "%s", dump)
		}
	}
	return resp, err
}
