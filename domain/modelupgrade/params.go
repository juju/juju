// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrade

import (
	"github.com/juju/juju/core/semversion"
)

// UpgradeModelParams holds the parameters used to upgrade a model.
type UpgradeModelParams struct {
	ControllerModelVersion semversion.Number
	TargetVersion          semversion.Number
	AgentStream            string
	IgnoreAgentVersions    bool
	DryRun                 bool
}
