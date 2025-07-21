// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg_test

import (
	"github.com/juju/charm/v12"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type imageSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&imageSuite{})

func (*imageSuite) TestGetJujuOCIImagePath(c *gc.C) {
	controllerCfg := testing.FakeControllerConfig()

	controllerCfg[controller.CAASImageRepo] = "testing-repo"
	ver := version.MustParse("2.6-beta3.666")
	path, err := podcfg.GetJujuOCIImagePath(controllerCfg, nil, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, jc.DeepEquals, "testing-repo/jujud-operator:2.6-beta3.666")

	controllerCfg[controller.CAASImageRepo] = "testing-repo:8080"
	ver = version.MustParse("2.6-beta3.666")
	path, err = podcfg.GetJujuOCIImagePath(controllerCfg, nil, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, jc.DeepEquals, "testing-repo:8080/jujud-operator:2.6-beta3.666")

	controllerCfg[controller.CAASOperatorImagePath] = "testing-old-repo/jujud-old-operator:1.6"
	ver = version.MustParse("2.6-beta3")
	path, err = podcfg.GetJujuOCIImagePath(controllerCfg, nil, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, jc.DeepEquals, "testing-old-repo/jujud-old-operator:2.6-beta3")

	modelCfg := testing.CustomModelConfig(c, map[string]interface{}{config.ModelCAASImageRepo: "testing-old-model-repo"})
	controllerCfg[controller.CAASOperatorImagePath] = "testing-old-controller-repo/jujud-old-operator:1.6"
	ver = version.MustParse("2.7-beta3")
	path, err = podcfg.GetJujuOCIImagePath(controllerCfg, modelCfg, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, jc.DeepEquals, "testing-old-model-repo/jujud-old-operator:2.7-beta3")
}

func (*imageSuite) TestRebuildOldOperatorImagePath(c *gc.C) {
	ver := version.MustParse("2.6-beta3")
	path, err := podcfg.RebuildOldOperatorImagePath("ghcr.io/juju/jujud-operator:666", ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, jc.DeepEquals, "ghcr.io/juju/jujud-operator:2.6-beta3")
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
	c.Assert(path, gc.DeepEquals, `ghcr.io/juju/charm-base:ubuntu-20.04`)

	path, err = podcfg.ImageForBase("", charm.Base{Name: "ubuntu", Channel: charm.Channel{
		Track: "20.04", Risk: charm.Edge,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.DeepEquals, `ghcr.io/juju/charm-base:ubuntu-20.04-edge`)
}

func (*imageSuite) TestRecoverRepoFromOperatorPath(c *gc.C) {
	repo, err := podcfg.RecoverRepoFromOperatorPath("testing-repo/jujud-operator:2.6-beta3.666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(repo, gc.Equals, "testing-repo")

	repo, err = podcfg.RecoverRepoFromOperatorPath("testing-repo:8080/jujud-operator:2.6-beta3.666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(repo, gc.Equals, "testing-repo:8080")

	repo, err = podcfg.RecoverRepoFromOperatorPath("ghcr.io/juju/jujud-operator:2.6-beta3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(repo, gc.Equals, "ghcr.io/juju")

	_, err = podcfg.RecoverRepoFromOperatorPath("ghcr.io/juju/nope:2.6-beta3")
	c.Assert(err, gc.ErrorMatches, `image path "ghcr.io/juju/nope:2.6-beta3" does not match the form somerepo/jujud-operator:\.\*`)
}
