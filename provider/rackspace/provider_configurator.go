// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"gopkg.in/goose.v1/nova"
)

type rackspaceProviderConfigurator struct{}

func (c *rackspaceProviderConfigurator) UseSecurityGroups() bool {
	return false
}

func (c *rackspaceProviderConfigurator) InitialNetworks() []nova.ServerNetworks {
	return []nova.ServerNetworks{
		{NetworkId: "00000000-0000-0000-0000-000000000000"},
		{NetworkId: "11111111-1111-1111-1111-111111111111"},
	}
}

func (c *rackspaceProviderConfigurator) ModifyRunServerOptions(options *nova.RunServerOpts) {
	options.ConfigDrive = true
}
