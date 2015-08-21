// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniteravailability

import (
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

func PatchedManifold(config ManifoldConfig, read readFunc, write writeFunc) dependency.Manifold {
	manifold := util.AgentManifold(
		util.AgentManifoldConfig(config),
		newWorker(read, write))
	manifold.Output = outputFunc
	return manifold
}
