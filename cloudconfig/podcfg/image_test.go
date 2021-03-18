// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/testing"
	"github.com/juju/systems"
	"github.com/juju/systems/channel"
)

type imageSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&imageSuite{})

func (*imageSuite) TestGetJujuOCIImagePath(c *gc.C) {
	cfg := testing.FakeControllerConfig()

	cfg[controller.CAASImageRepo] = "testing-repo"
	ver := version.MustParse("2.6-beta3")
	path := podcfg.GetJujuOCIImagePath(cfg, ver, 666)
	c.Assert(path, jc.DeepEquals, "testing-repo/jujud-operator:2.6-beta3.666")

	cfg[controller.CAASOperatorImagePath] = "testing-old-repo/jujud-old-operator:1.6"
	path = podcfg.GetJujuOCIImagePath(cfg, ver, 0)
	c.Assert(path, jc.DeepEquals, "testing-old-repo/jujud-old-operator:2.6-beta3")
}

func (*imageSuite) TestImageForBase(c *gc.C) {
	_, err := podcfg.ImageForBase("", systems.Base{})
	c.Assert(err, gc.ErrorMatches, `base name not valid`)

	_, err = podcfg.ImageForBase("", systems.Base{Name: "ubuntu"})
	c.Assert(err, gc.ErrorMatches, `channel "" not valid`)

	_, err = podcfg.ImageForBase("", systems.Base{Name: "ubuntu", Channel: channel.Channel{
		Track: "20.04",
	}})
	c.Assert(err, gc.ErrorMatches, `channel "" not valid`)

	path, err := podcfg.ImageForBase("", systems.Base{Name: "ubuntu", Channel: channel.Channel{
		Track: "20.04", Risk: channel.Stable,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.DeepEquals, `jujusolutions/ubuntu:20.04`)

	path, err = podcfg.ImageForBase("", systems.Base{Name: "ubuntu", Channel: channel.Channel{
		Track: "20.04", Risk: channel.Edge,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.DeepEquals, `jujusolutions/ubuntu:20.04-edge`)
}
