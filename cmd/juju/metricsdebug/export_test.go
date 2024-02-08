// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"errors"

	"github.com/juju/clock"
	"github.com/juju/cmd/v4"

	"github.com/juju/juju/api"
	actionapi "github.com/juju/juju/api/client/action"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

var (
	NewClient    = &newClient
	NewRunClient = &newRunClient
	NewAPIConn   = &newAPIConn
)

// NewRunClientFnc returns a function that returns a struct that implements the
// runClient interface. This function can be used to patch the NewRunClient
// variable in tests.
func NewRunClientFnc(client runClient) func(api.Connection) runClient {
	return func(_ api.Connection) runClient {
		return client
	}
}

func PatchGetActionResult(patchValue func(interface{}, interface{}), actions map[string]actionapi.ActionResult) {
	patchValue(&getActionResult, func(_ runClient, id string, _ clock.Clock, _ clock.Timer) (actionapi.ActionResult, error) {
		if res, ok := actions[id]; ok {
			return res, nil
		}
		return actionapi.ActionResult{}, errors.New("plm")
	})
}

func NewCollectMetricsCommandForTest() cmd.Command {
	cmd := &collectMetricsCommand{}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(cmd)
}

func NewMetricsCommandForTest() cmd.Command {
	cmd := &MetricsCommand{}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(cmd)
}
