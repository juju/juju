// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type ConfigSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) TestDefaultsPassValidation(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "rackspace",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	valid, err := providerInstance.Validate(cfg, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(valid.Type(), gc.Equals, "rackspace")
	c.Assert(valid.Name(), gc.Equals, "testenv")
}
