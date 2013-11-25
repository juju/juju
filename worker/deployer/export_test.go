// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state/api/params"
)

type fakeAPI struct{}

func (*fakeAPI) ConnectionInfo() (params.DeployerConnectionValues, error) {
	return params.DeployerConnectionValues{
		[]string{"s1:123", "s2:123"},
		[]string{"a1:123", "a2:123"},
		2345,
	}, nil
}

func NewTestSimpleContext(agentConfig agent.Config, initDir, logDir, syslogConfigDir string) *SimpleContext {
	return &SimpleContext{
		api:             &fakeAPI{},
		agentConfig:     agentConfig,
		initDir:         initDir,
		logDir:          logDir,
		syslogConfigDir: syslogConfigDir,
	}
}
