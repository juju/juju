// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"launchpad.net/juju-core/agent"
)

type fakeAddresser struct{}

func (*fakeAddresser) StateAddresses() ([]string, error) {
	return []string{"s1:123", "s2:123"}, nil
}

func (*fakeAddresser) APIAddresses() ([]string, error) {
	return []string{"a1:123", "a2:123"}, nil
}

func NewTestSimpleContext(agentConfig agent.Config, initDir, logDir, syslogConfigDir string) *SimpleContext {
	return &SimpleContext{
		addresser:       &fakeAddresser{},
		agentConfig:     agentConfig,
		initDir:         initDir,
		logDir:          logDir,
		syslogConfigDir: syslogConfigDir,
	}
}
