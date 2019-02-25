// Copyright 2019 Canonical Ltd.
// Copyright 2019 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import "github.com/juju/juju/cmd/jujud/agent/caasoperator"

var (
	// Should be an explicit dependency, can't do it cleanly yet.
	// Exported for testing.
	CaasOperatorManifolds = caasoperator.Manifolds
)
