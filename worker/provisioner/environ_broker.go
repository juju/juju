// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/juju-core/environs"
)

func newEnvironBroker(environ environs.Environ) Broker {
	return &environBroker{environ}
}

type environBroker struct {
	environs.Environ
}

// Defer to the Environ for:
//   StartInstance
//   StopInstances
//   AllInstances
