// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
    "launchpad.net/juju-core/api/upgrader"
    )


func NewUpgradeHandler(apiUpgrader *upgrader.Upgrader, agentTag string)
	agentTag    string
        toolManager agent.ToolManager
}
