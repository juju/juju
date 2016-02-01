// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
)

// NewSetCommand returns a SetCommand with the api provided as specified.
func NewSetCommandWithAPI(clientAPI ClientAPI, serviceAPI ServiceAPI) cmd.Command {
	return envcmd.Wrap(&setCommand{
		clientApi:  clientAPI,
		serviceApi: serviceAPI,
	})
}

// NewGetCommand returns a GetCommand with the api provided as specified.
func NewGetCommandForTest(api GetServiceAPI) cmd.Command {
	return envcmd.Wrap(&getCommand{
		api: api,
	})
}

// NewAddUnitCommand returns an AddUnitCommand with the api provided as specified.
func NewAddUnitCommandForTest(api ServiceAddUnitAPI) cmd.Command {
	return envcmd.Wrap(&addUnitCommand{
		api: api,
	})
}
