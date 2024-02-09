// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracing

import (
	"net/http"
	"net/http/httputil"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/juju/loggo/v2"
)

type LoggingPolicy struct {
	Logger loggo.Logger
}

func (p *LoggingPolicy) Do(req *policy.Request) (*http.Response, error) {
	if p.Logger.IsTraceEnabled() {
		dump, err := httputil.DumpRequest(req.Raw(), true)
		if err != nil {
			p.Logger.Tracef("failed to dump request: %v", err)
			p.Logger.Tracef("%+v", req.Raw())
		} else {
			p.Logger.Tracef("%s", dump)
		}
	}
	resp, err := req.Next()
	if err == nil && p.Logger.IsTraceEnabled() {
		dump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			p.Logger.Tracef("failed to dump response: %v", err)
			p.Logger.Tracef("%+v", resp)
		} else {
			p.Logger.Tracef("%s", dump)
		}
	}
	return resp, err
}
