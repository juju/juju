// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg_test

import (
	"github.com/juju/charm/v9"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/testing"
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

func (*imageSuite) TestGetJujuOCIImagePathWithLocalRepo(c *gc.C) {
	cfg := testing.FakeControllerConfig()
	ver := version.MustParse("2.6-beta3")

	cfg[controller.CAASImageRepo] = "192.168.1.1/testing-repo"
	path := podcfg.GetJujuOCIImagePath(cfg, ver, 666)
	c.Assert(path, jc.DeepEquals, "192.168.1.1/testing-repo/jujud-operator:2.6-beta3.666")

	cfg[controller.CAASImageRepo] = "192.168.1.1:8890/testing-repo"
	path = podcfg.GetJujuOCIImagePath(cfg, ver, 666)
	c.Assert(path, jc.DeepEquals, "192.168.1.1:8890/testing-repo/jujud-operator:2.6-beta3.666")

	cfg[controller.CAASOperatorImagePath] = "192.168.1.1/testing-old-repo/jujud-old-operator:1.6"
	path = podcfg.GetJujuOCIImagePath(cfg, ver, 0)
	c.Assert(path, jc.DeepEquals, "192.168.1.1/testing-old-repo/jujud-old-operator:2.6-beta3")

	cfg[controller.CAASOperatorImagePath] = "192.168.1.1:8890/testing-old-repo/jujud-old-operator:1.6"
	path = podcfg.GetJujuOCIImagePath(cfg, ver, 0)
	c.Assert(path, jc.DeepEquals, "192.168.1.1:8890/testing-old-repo/jujud-old-operator:2.6-beta3")

	cfg[controller.CAASOperatorImagePath] = "jujud-old-operator:1.6"
	path = podcfg.GetJujuOCIImagePath(cfg, ver, 0)
	c.Assert(path, jc.DeepEquals, "jujud-old-operator:2.6-beta3")
}

func (*imageSuite) TestImageForBase(c *gc.C) {
	_, err := podcfg.ImageForBase("", charm.Base{})
	c.Assert(err, gc.ErrorMatches, `empty base name not valid`)

	_, err = podcfg.ImageForBase("", charm.Base{Name: "ubuntu"})
	c.Assert(err, gc.ErrorMatches, `channel "" not valid`)

	_, err = podcfg.ImageForBase("", charm.Base{Name: "ubuntu", Channel: charm.Channel{
		Track: "20.04",
	}})
	c.Assert(err, gc.ErrorMatches, `channel "20.04/" not valid`)

	path, err := podcfg.ImageForBase("", charm.Base{Name: "ubuntu", Channel: charm.Channel{
		Track: "20.04", Risk: charm.Stable,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.DeepEquals, `jujusolutions/charm-base:ubuntu-20.04`)

	path, err = podcfg.ImageForBase("", charm.Base{Name: "ubuntu", Channel: charm.Channel{
		Track: "20.04", Risk: charm.Edge,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.DeepEquals, `jujusolutions/charm-base:ubuntu-20.04-edge`)
}
