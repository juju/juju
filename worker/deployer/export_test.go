// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
)

type fakeAPI struct{}

func (*fakeAPI) ConnectionInfo() (params.DeployerConnectionValues, error) {
	return params.DeployerConnectionValues{
		StateAddresses: []string{"s1:123", "s2:123"},
		APIAddresses:   []string{"a1:123", "a2:123"},
	}, nil
}

func NewTestSimpleContext(c *gc.C, agentConfig agent.Config, services services) *SimpleContext {
	return &SimpleContext{
		api:         &fakeAPI{},
		agentConfig: agentConfig,
		services:    services,
	}
}
