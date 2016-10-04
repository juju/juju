// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"strings"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/manual/linux"
	"github.com/juju/juju/testing"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"
)

type generateSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&generateSuite{})

func (g *generateSuite) TestScript(c *gc.C) {
	icfg := &instancecfg.InstanceConfig{}
	sup := series.SupportedSeries()
	ok := false
	for _, val := range sup {
		if val == "genericlinux" {
			continue
		}
		if strings.Contains(val, "win") {
			continue
		}
		icfg.Series = val
		prov, err := manual.NewScriptProvisioner(icfg)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(prov, gc.NotNil)
		switch prov.(type) {
		default:
			ok = false
		case *linux.Script:
			ok = true
		}
		c.Assert(ok, jc.IsTrue)
		c.Assert(val, gc.NotNil)
	}
	// this is supposed to fail
	icfg.Series = "MyFunckyOS"
	prov, err := manual.NewScriptProvisioner(icfg)
	c.Assert(err, gc.NotNil)
	c.Assert(prov, gc.IsNil)
}
