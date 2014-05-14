// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

type configSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&configSuite{})

// copyAttrs copies values from src into dest.  If src contains a key that was
// already in dest, its value in dest will still be updated to the one from
// src.
func copyAttrs(src, dest map[string]interface{}) {
	for k, v := range src {
		dest[k] = v
	}
}

// newConfig creates a MAAS environment config from attributes.
func newConfig(values map[string]interface{}) (*maasEnvironConfig, error) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"name": "testenv",
		"type": "maas",
	}).Merge(values)
	env, err := environs.NewFromAttrs(attrs)
	if err != nil {
		return nil, err
	}
	return env.(*maasEnviron).ecfg(), nil
}

func (*configSuite) TestParsesMAASSettings(c *gc.C) {
	server := "http://maas.testing.invalid/maas/"
	oauth := "consumer-key:resource-token:resource-secret"
	future := "futurama"
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	ecfg, err := newConfig(map[string]interface{}{
		"maas-server":     server,
		"maas-oauth":      oauth,
		"maas-agent-name": uuid.String(),
		"future-key":      future,
	})
	c.Assert(err, gc.IsNil)
	c.Check(ecfg.maasServer(), gc.Equals, server)
	c.Check(ecfg.maasOAuth(), gc.DeepEquals, oauth)
	c.Check(ecfg.maasAgentName(), gc.Equals, uuid.String())
	c.Check(ecfg.UnknownAttrs()["future-key"], gc.DeepEquals, future)
}

func (*configSuite) TestMaasAgentNameDefault(c *gc.C) {
	ecfg, err := newConfig(map[string]interface{}{
		"maas-server": "http://maas.testing.invalid/maas/",
		"maas-oauth":  "consumer-key:resource-token:resource-secret",
	})
	c.Assert(err, gc.IsNil)
	c.Check(ecfg.maasAgentName(), gc.Equals, "")
}

func (*configSuite) TestChecksWellFormedMaasServer(c *gc.C) {
	_, err := newConfig(map[string]interface{}{
		"maas-server": "This should have been a URL.",
		"maas-oauth":  "consumer-key:resource-token:resource-secret",
	})
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, ".*malformed maas-server.*")
}

func (*configSuite) TestChecksWellFormedMaasOAuth(c *gc.C) {
	_, err := newConfig(map[string]interface{}{
		"maas-server": "http://maas.testing.invalid/maas/",
		"maas-oauth":  "This should have been a 3-part token.",
	})
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, ".*malformed maas-oauth.*")
}

func (*configSuite) TestValidateUpcallsEnvironsConfigValidate(c *gc.C) {
	// The base Validate() function will not allow an environment to
	// change its name.  Trigger that error so as to prove that the
	// environment provider's Validate() calls the base Validate().
	baseAttrs := map[string]interface{}{
		"maas-server": "http://maas.testing.invalid/maas/",
		"maas-oauth":  "consumer-key:resource-token:resource-secret",
	}
	oldCfg, err := newConfig(baseAttrs)
	c.Assert(err, gc.IsNil)
	newName := oldCfg.Name() + "-but-different"
	newCfg, err := oldCfg.Apply(map[string]interface{}{"name": newName})
	c.Assert(err, gc.IsNil)

	_, err = maasEnvironProvider{}.Validate(newCfg, oldCfg.Config)

	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, ".*cannot change name.*")
}

func (*configSuite) TestValidateCannotChangeAgentName(c *gc.C) {
	baseAttrs := map[string]interface{}{
		"maas-server":     "http://maas.testing.invalid/maas/",
		"maas-oauth":      "consumer-key:resource-token:resource-secret",
		"maas-agent-name": "1234-5678",
	}
	oldCfg, err := newConfig(baseAttrs)
	c.Assert(err, gc.IsNil)
	newCfg, err := oldCfg.Apply(map[string]interface{}{
		"maas-agent-name": "9876-5432",
	})
	c.Assert(err, gc.IsNil)
	_, err = maasEnvironProvider{}.Validate(newCfg, oldCfg.Config)
	c.Assert(err, gc.ErrorMatches, "cannot change maas-agent-name")
}
