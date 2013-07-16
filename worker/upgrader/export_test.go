// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"io"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/version"
)

func NewUpgradeHandler(apiState *api.State, agentTag string) *upgradeHandler {
	return &upgradeHandler{
		apiState:    apiState,
		agentTag:    agentTag,
		toolManager: NilToolsManager{},
	}
}

type NilToolsManager struct {
}

func (n NilToolsManager) ReadTools(version version.Binary) (*agent.Tools, error) {
	return nil, nil
}

func (n NilToolsManager) UnpackTools(tools *agent.Tools, r io.Reader) error {
	return nil
}
