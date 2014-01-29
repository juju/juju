// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package null_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/provider/null"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

type providerSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) TestPrepare(c *gc.C) {
	minimal := null.MinimalConfigValues()
	delete(minimal, "storage-auth-key")
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, gc.IsNil)
	env, err := null.ProviderInstance.Prepare(testConfig)
	c.Assert(err, gc.IsNil)
	cfg := env.Config()
	key, _ := cfg.UnknownAttrs()["storage-auth-key"].(string)
	c.Assert(key, jc.Satisfies, utils.IsValidUUIDString)
}
