// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/unit"
)

var _ = gc.Suite(&CommonManifoldSuite{
	manifold: unit.RsyslogUpdaterManifold(
		unit.RsyslogUpdaterManifoldConfig{
			AgentName:     "agent-name",
			ApiCallerName: "api-caller-name",
		},
	),
	newWorker: unit.NewRsyslogUpdater,
})
