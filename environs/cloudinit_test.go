// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type CloudInitSuite struct{}

var _ = Suite(&CloudInitSuite{})

func (s *CloudInitSuite) TestFinishInstanceConfig(c *C) {
	cfg, err := config.New(map[string]interface{}{
		"name":            "barbara",
		"type":            "dummy",
		"authorized-keys": "we-are-the-keys",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, IsNil)
	mcfg := &cloudinit.MachineConfig{
		StateInfo: &state.Info{Tag: "not touched"},
		APIInfo:   &api.Info{Tag: "not touched"},
	}
	err = environs.FinishMachineConfig(mcfg, cfg, constraints.Value{})
	c.Assert(err, IsNil)
	c.Assert(mcfg, DeepEquals, &cloudinit.MachineConfig{
		AuthorizedKeys: "we-are-the-keys",
		ProviderType:   "dummy",
		StateInfo:      &state.Info{Tag: "not touched"},
		APIInfo:        &api.Info{Tag: "not touched"},
	})
}

func (s *CloudInitSuite) TestFinishBootstrapConfig(c *C) {
	cfg, err := config.New(map[string]interface{}{
		"name":            "barbara",
		"type":            "dummy",
		"admin-secret":    "lisboan-pork",
		"authorized-keys": "we-are-the-keys",
		"agent-version":   "1.2.3",
		"ca-cert":         testing.CACert,
		"ca-private-key":  testing.CAKey,
		"state-server":    false,
		"secret":          "british-horse",
	})
	c.Assert(err, IsNil)
	oldAttrs := cfg.AllAttrs()
	mcfg := &cloudinit.MachineConfig{
		StateServer: true,
	}
	cons := constraints.MustParse("mem=1T cpu-power=999999999")
	err = environs.FinishMachineConfig(mcfg, cfg, cons)
	c.Check(err, IsNil)
	c.Check(mcfg.AuthorizedKeys, Equals, "we-are-the-keys")
	password := utils.PasswordHash("lisboan-pork")
	c.Check(mcfg.APIInfo, DeepEquals, &api.Info{
		Password: password, CACert: []byte(testing.CACert),
	})
	c.Check(mcfg.StateInfo, DeepEquals, &state.Info{
		Password: password, CACert: []byte(testing.CACert),
	})
	c.Check(mcfg.StatePort, Equals, cfg.StatePort())
	c.Check(mcfg.APIPort, Equals, cfg.APIPort())
	c.Check(mcfg.Constraints, DeepEquals, cons)

	oldAttrs["ca-private-key"] = ""
	oldAttrs["admin-secret"] = ""
	delete(oldAttrs, "secret")
	c.Check(mcfg.Config.AllAttrs(), DeepEquals, oldAttrs)
	srvCertPEM := mcfg.StateServerCert
	srvKeyPEM := mcfg.StateServerKey
	_, _, err = cert.ParseCertAndKey(srvCertPEM, srvKeyPEM)
	c.Check(err, IsNil)

	err = cert.Verify(srvCertPEM, []byte(testing.CACert), time.Now())
	c.Assert(err, IsNil)
	err = cert.Verify(srvCertPEM, []byte(testing.CACert), time.Now().AddDate(9, 0, 0))
	c.Assert(err, IsNil)
	err = cert.Verify(srvCertPEM, []byte(testing.CACert), time.Now().AddDate(10, 0, 1))
	c.Assert(err, NotNil)
}
