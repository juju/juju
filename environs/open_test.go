// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/testing"
)

type OpenSuite struct{}

var _ = Suite(&OpenSuite{})

func (OpenSuite) TearDownTest(c *C) {
	dummy.Reset()
}

func (OpenSuite) TestNewDummyEnviron(c *C) {
	// matches *Settings.Map()
	config := map[string]interface{}{
		"name":            "foo",
		"type":            "dummy",
		"state-server":    false,
		"authorized-keys": "i-am-a-key",
		"admin-secret":    "foo",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	}
	env, err := environs.NewFromAttrs(config)
	c.Assert(err, IsNil)
	c.Assert(env.Bootstrap(constraints.Value{}), IsNil)
}

func (OpenSuite) TestNewUnknownEnviron(c *C) {
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "foo",
		"type":            "wondercloud",
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, ErrorMatches, "no registered provider for.*")
	c.Assert(env, IsNil)
}

func (OpenSuite) TestNewFromNameNoDefault(c *C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfigNoDefault, testing.SampleCertName).Restore()

	_, err := environs.NewFromName("")
	c.Assert(err, ErrorMatches, "no default environment found")
}

func (OpenSuite) TestNewFromNameGetDefault(c *C) {
	defer testing.MakeFakeHome(c, testing.SingleEnvConfig, testing.SampleCertName).Restore()

	e, err := environs.NewFromName("")
	c.Assert(err, IsNil)
	c.Assert(e.Name(), Equals, "erewhemos")
}
