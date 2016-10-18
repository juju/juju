// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"errors"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
)

var (
	NewClient        = &newClient
	NewRunClient     = &newRunClient
	NewServiceClient = &newServiceClient
	NewAPIConn       = &newAPIConn
)

// NewRunClientFnc returns a function that returns a struct that implements the
// runClient interface. This function can be used to patch the NewRunClient
// variable in tests.
func NewRunClientFnc(client runClient) func(api.Connection) runClient {
	return func(_ api.Connection) runClient {
		return client
	}
}

// NewServiceClientFnc returns a function that returns a struct that implements the
// serviceClient interface. This function can be used to patch the NewServiceClient
// variable in tests.
func NewServiceClientFnc(client serviceClient) func(api.Connection) serviceClient {
	return func(_ api.Connection) serviceClient {
		return client
	}
}

func PatchGetActionResult(patchValue func(interface{}, interface{}), actions map[string]params.ActionResult) {
	patchValue(&getActionResult, func(_ runClient, id string, _ *time.Timer) (params.ActionResult, error) {
		if res, ok := actions[id]; ok {
			return res, nil
		}
		return params.ActionResult{}, errors.New("plm")
	})
}
