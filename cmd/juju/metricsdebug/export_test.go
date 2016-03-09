// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"github.com/juju/juju/cmd/modelcmd"
)

var (
	NewClient    = &newClient
	NewRunClient = &newRunClient
)

// NewRunClientFnc returns a function that returns a struct that implements the
// runClient interface. This function can be used to patch the NewRunClient
// variable in tests.
func NewRunClientFnc(client runClient) func(modelcmd.ModelCommandBase) (runClient, error) {
	return func(_ modelcmd.ModelCommandBase) (runClient, error) {
		return client, nil
	}
}
