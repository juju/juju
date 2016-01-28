// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
)

// NewSetCommandForTest returns a SetCommand with the api provided as specified.
func NewSetCommandForTest(serviceAPI serviceAPI) cmd.Command {
	return envcmd.Wrap(&setCommand{
		serviceApi: serviceAPI,
	})
}

// NewUnsetCommand returns an UnsetCommand with the api provided as specified.
func NewUnsetCommand(api UnsetServiceAPI) cmd.Command {
	return envcmd.Wrap(&unsetCommand{
		api: api,
	})
}

// NewGetCommand returns a GetCommand with the api provided as specified.
func NewGetCommand(api getServiceAPI) cmd.Command {
	return envcmd.Wrap(&getCommand{
		api: api,
	})
}

// NewAddUnitCommand returns an AddUnitCommand with the api provided as specified.
func NewAddUnitCommand(api serviceAddUnitAPI) cmd.Command {
	return envcmd.Wrap(&addUnitCommand{
		api: api,
	})
}

var (
	NewServiceSetConstraintsCommand = newServiceSetConstraintsCommand
)
