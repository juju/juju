// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"errors"
	"time"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

var (
	NewClient        = &newClient
	NewRunClient     = &newRunClient
	NewServiceClient = &newServiceClient
)

// NewRunClientFnc returns a function that returns a struct that implements the
// runClient interface. This function can be used to patch the NewRunClient
// variable in tests.
func NewRunClientFnc(client runClient) func(modelcmd.ModelCommandBase) (runClient, error) {
	return func(_ modelcmd.ModelCommandBase) (runClient, error) {
		return client, nil
	}
}

// NewServiceClientFnc returns a function that returns a struct that implements the
// serviceClient interface. This function can be used to patch the NewServiceClient
// variable in tests.
func NewServiceClientFnc(client serviceClient) func(modelcmd.ModelCommandBase) (serviceClient, error) {
	return func(_ modelcmd.ModelCommandBase) (serviceClient, error) {
		return client, nil
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
