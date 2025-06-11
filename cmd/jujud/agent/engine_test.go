// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/cmd/jujud/agent/machine"
)

type MachineManifoldsFunc func(config machine.ManifoldsConfig) dependency.Manifolds
