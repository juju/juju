// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/cloudinit"
	jujuos "github.com/juju/juju/core/os"
	"github.com/juju/juju/provider/oracle"
)

type userdataSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&userdataSuite{})

func (s *userdataSuite) TestRedner(c *gc.C) {
	renderer := oracle.OracleRenderer{}
	cfg, err := cloudinit.New("trusty")
	c.Assert(err, gc.IsNil)
	c.Assert(cfg, gc.NotNil)

	_, err = renderer.Render(cfg, jujuos.Ubuntu)
	c.Assert(err, gc.IsNil)
}

func (s *userdataSuite) TestRenderWithErrors(c *gc.C) {
	renderer := oracle.OracleRenderer{}
	cfg, err := cloudinit.New("trusty")
	c.Assert(err, gc.IsNil)
	c.Assert(cfg, gc.NotNil)

	for _, val := range []jujuos.OSType{
		jujuos.Windows,
		jujuos.CentOS,
		jujuos.Unknown,
		jujuos.OSX,
	} {
		_, err := renderer.Render(cfg, val)
		c.Assert(err, gc.NotNil)
	}
}
