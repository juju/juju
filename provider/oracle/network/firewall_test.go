// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/oracle/network"
	"github.com/juju/juju/testing"
	gc "gopkg.in/check.v1"
)

type firewallSuite struct{}

var _ = gc.Suite(&firewallSuite{})

type FakeEnvironConfig struct {
	cfg *config.Config
}

func (f *FakeEnvironConfig) Config() *config.Config {
	return f.cfg
}

func (u *firewallSuite) TestNewFirewall(c *gc.C) {
	firewall := network.NewFirewall(nil, nil)
	c.Assert(firewall, gc.NotNil)

	cfg := &FakeEnvironConfig{cfg: testing.ModelConfig(c)}
	cli := &api.Client{}
	firewall = network.NewFirewall(cfg, cli)
	c.Assert(firewall, gc.NotNil)
}
