// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state/api/params"
)

type fakeAddresser struct{}

func (*fakeAddresser) StateAddresses() ([]string, error) {
	return []string{"s1:123", "s2:123"}, nil
}

func (*fakeAddresser) APIAddresses() ([]string, error) {
	return []string{"a1:123", "a2:123"}, nil
}

func (*fakeAddresser) ServerAddresses() (params.ServerAddressesResults, error) {
	return params.ServerAddressesResults{
		[]string{"s1:123", "s2:123"},
		[]string{"a1:123", "a2:123"},
		2345,
	}, nil
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
