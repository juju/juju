// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
)

func newEnvironBroker(environ environs.Environ, state *state.State) Broker {
	return &environBroker{environ, state}
}

type environBroker struct {
	environs.Environ
	*state.State
}

// Defer to the Environ for:
//   StartInstance
//   StopInstances
//   AllInstances

// Defer to State for
//   AllMachines

// TODO(thumper): the AllMachines method will need tweaking once we have
// containers in state in order to filter out the containers and only return
// the top level machines.
