// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"github.com/juju/version"

	"github.com/juju/juju/agent/tools"
)

// UpgradeReadyError is returned by an Upgrader to report that
// an upgrade is ready to be performed and a restart is due.
type UpgradeReadyError struct {
	AgentName string
	OldTools  version.Binary
	NewTools  version.Binary
	DataDir   string
}

func (e *UpgradeReadyError) Error() string {
	return "must restart: an agent upgrade is available"
}

// ChangeAgentTools does the actual agent upgrade.
// It should be called just before an agent exits, so that
// it will restart running the new tools.
func (e *UpgradeReadyError) ChangeAgentTools() error {
	agentTools, err := tools.ChangeAgentTools(e.DataDir, e.AgentName, e.NewTools)
	if err != nil {
		return err
	}
	logger.Infof("upgraded from %v to %v (%q)", e.OldTools, agentTools.Version, agentTools.URL)
	return nil
}
