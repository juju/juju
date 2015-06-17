// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/service/common"
	svctesting "github.com/juju/juju/service/common/testing"
)

type fakeAPI struct{}

func (*fakeAPI) ConnectionInfo() (params.DeployerConnectionValues, error) {
	return params.DeployerConnectionValues{
		StateAddresses: []string{"s1:123", "s2:123"},
		APIAddresses:   []string{"a1:123", "a2:123"},
	}, nil
}

func NewTestSimpleContext(agentConfig agent.Config, logDir string, data *svctesting.FakeServiceData) *SimpleContext {
	return &SimpleContext{
		api:         &fakeAPI{},
		agentConfig: agentConfig,
		discoverService: func(name string, conf common.Conf) (deployerService, error) {
			svc := svctesting.NewFakeService(name, conf)
			svc.FakeServiceData = data
			return svc, nil
		},
		listServices: func() ([]string, error) {
			return data.InstalledNames(), nil
		},
	}
}
