// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_client_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/lxd/lxd_client"
)

type configSuite struct{}

var _ = gc.Suite(&configSuite{})

func (*configSuite) TestValidateValid(c *gc.C) {
	cfg := lxd_client.Config{
		Namespace: "spam",
		Remote:    "eggs",
	}
	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (*configSuite) TestValidateMissingNamespace(c *gc.C) {
	cfg := lxd_client.Config{
		Remote: "eggs",
	}
	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (*configSuite) TestValidateMissingRemote(c *gc.C) {
	cfg := lxd_client.Config{
		Namespace: "spam",
	}
	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}
