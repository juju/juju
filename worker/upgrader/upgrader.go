// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"fmt"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/upgrader"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.upgrader")

type upgradeHandler struct {
	apiState    *api.State
	apiUpgrader *upgrader.Upgrader
	agentTag    string
	toolManager agent.ToolsManager
}

func NewUpgrader(st *api.State, agentTag string, toolManager agent.ToolsManager) worker.NotifyWorker {
	return worker.NewNotifyWorker(&upgradeHandler{
		apiState:    st,
		agentTag:    agentTag,
		toolManager: toolManager,
	})
}

func (u *upgradeHandler) String() string {
	return fmt.Sprintf("upgrader for %q", u.agentTag)
}

func (u *upgradeHandler) SetUp() (api.NotifyWatcher, error) {
	// First thing we do is alert the API of our current tools
	u.apiUpgrader = u.apiState.Upgrader()
	cur := version.Current
	err := u.apiUpgrader.SetTools(params.AgentTools{
		Tag:    u.agentTag,
		Major:  cur.Major,
		Minor:  cur.Minor,
		Patch:  cur.Patch,
		Build:  cur.Build,
		Arch:   cur.Arch,
		Series: cur.Series,
		URL:    "",
	})
	if err != nil {
		return nil, err
	}
	return u.apiUpgrader.WatchAPIVersion(u.agentTag)
}

func (u *upgradeHandler) TearDown() error {
	u.apiUpgrader = nil
	u.apiState = nil
	return nil
}

func (u *upgradeHandler) Handle() error {
	requestedTools, err := u.apiUpgrader.Tools(u.agentTag)
	return nil
}
