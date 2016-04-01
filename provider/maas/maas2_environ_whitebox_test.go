// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
)

type maas2EnvironSuite struct {
	controllerSuite
}

var _ = gc.Suite(&maas2EnvironSuite{})

func (suite *maas2EnvironSuite) TestNewEnvironWithController(c *gc.C) {
	testAttrs := coretesting.Attrs{}
	for k, v := range maasEnvAttrs {
		testAttrs[k] = v
	}
	testAttrs["maas-server"] = suite.testServer.Server.URL
	attrs := coretesting.FakeConfig().Merge(testAttrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	env, err := NewEnviron(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
}
