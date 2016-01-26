// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"github.com/juju/juju/cmd/envcmd"
)

var (
	NewClient    = &newClient
	NewRunClient = &newRunClient
)

// NewRunClientFnc returns a function that returns a struct that implements the
// runClient interface. This function can be used to patch the NewRunClient
// variable in tests.
func NewRunClientFnc(client runClient) func(envcmd.EnvCommandBase) (runClient, error) {
	return func(_ envcmd.EnvCommandBase) (runClient, error) {
		return client, nil
	}
}
