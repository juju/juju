// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"net/http"
	"net/http/httputil"

	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest"
	"github.com/juju/loggo"
)

// tracingPrepareDecorator returns an autorest.PrepareDecorator that
// logs requests at trace level.
func tracingPrepareDecorator(logger loggo.Logger) autorest.PrepareDecorator {
	return func(p autorest.Preparer) autorest.Preparer {
		return autorest.PreparerFunc(func(r *http.Request) (*http.Request, error) {
			dump, err := httputil.DumpRequest(r, true)
			if err != nil {
				logger.Tracef("failed to dump request: %v", err)
				logger.Tracef("%+v", r)
			} else {
				logger.Tracef("%s", dump)
			}
			return p.Prepare(r)
		})
	}
}

// tracingRespondDecorator returns an autorest.RespondDecorator that
// logs responses at trace level.
func tracingRespondDecorator(logger loggo.Logger) autorest.RespondDecorator {
	return func(r autorest.Responder) autorest.Responder {
		return autorest.ResponderFunc(func(resp *http.Response) error {
			dump, err := httputil.DumpResponse(resp, true)
			if err != nil {
				logger.Tracef("failed to dump response: %v", err)
				logger.Tracef("%+v", resp)
			} else {
				logger.Tracef("%s", dump)
			}
			return r.Respond(resp)
		})
	}
}
