// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/loggo"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
)

func InstanceTypeUnsupported(logger loggo.Logger, e environs.Environ, cons constraints.Value) {
	if cons.HasInstanceType() {
		logger.Warningf("instance-type constraint %s not supported for %s provider %q",
			cons.InstanceType, e.Config().Type(), e.Name())
	}
}
