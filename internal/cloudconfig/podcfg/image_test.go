// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/testing"
)

type imageSuite struct {
	testing.BaseSuite
}

func TestImageSuite(t *stdtesting.T) {
	tc.Run(t, &imageSuite{})
}

func (*imageSuite) TestGetJujuOCIImagePath(c *tc.C) {
	cfg := testing.FakeControllerConfig()

	cfg[controller.CAASImageRepo] = "testing-repo"
	ver := semversion.MustParse("2.6-beta3.666")
	path, err := podcfg.GetJujuOCIImagePath(cfg, ver)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path, tc.DeepEquals, "testing-repo/jujud-operator:2.6-beta3.666")

	cfg[controller.CAASImageRepo] = "testing-repo:8080"
	ver = semversion.MustParse("2.6-beta3.666")
	path, err = podcfg.GetJujuOCIImagePath(cfg, ver)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path, tc.DeepEquals, "testing-repo:8080/jujud-operator:2.6-beta3.666")

	cfg[controller.CAASOperatorImagePath] = "testing-old-repo/jujud-old-operator:1.6"
	ver = semversion.MustParse("2.6-beta3")
	path, err = podcfg.GetJujuOCIImagePath(cfg, ver)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path, tc.DeepEquals, "testing-old-repo/jujud-old-operator:2.6-beta3")
}

func (*imageSuite) TestRebuildOldOperatorImagePath(c *tc.C) {
	ver := semversion.MustParse("2.6-beta3")
	path, err := podcfg.RebuildOldOperatorImagePath("docker.io/jujusolutions/jujud-operator:666", ver)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path, tc.DeepEquals, "docker.io/jujusolutions/jujud-operator:2.6-beta3")
}

func (*imageSuite) TestImageForBase(c *tc.C) {
	_, err := podcfg.ImageForBase("", charm.Base{})
	c.Assert(err, tc.ErrorMatches, `empty base name not valid`)

	_, err = podcfg.ImageForBase("", charm.Base{Name: "ubuntu"})
	c.Assert(err, tc.ErrorMatches, `channel "" not valid`)

	_, err = podcfg.ImageForBase("", charm.Base{Name: "ubuntu", Channel: charm.Channel{
		Track: "20.04",
	}})
	c.Assert(err, tc.ErrorMatches, `channel "20.04/" not valid`)

	path, err := podcfg.ImageForBase("", charm.Base{Name: "ubuntu", Channel: charm.Channel{
		Track: "20.04", Risk: charm.Stable,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path, tc.DeepEquals, `docker.io/jujusolutions/charm-base:ubuntu-20.04`)

	path, err = podcfg.ImageForBase("", charm.Base{Name: "ubuntu", Channel: charm.Channel{
		Track: "20.04", Risk: charm.Edge,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path, tc.DeepEquals, `docker.io/jujusolutions/charm-base:ubuntu-20.04-edge`)
}

func (*imageSuite) TestRecoverRepoFromOperatorPath(c *tc.C) {
	repo, err := podcfg.RecoverRepoFromOperatorPath("testing-repo/jujud-operator:2.6-beta3.666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(repo, tc.Equals, "testing-repo")

	repo, err = podcfg.RecoverRepoFromOperatorPath("testing-repo:8080/jujud-operator:2.6-beta3.666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(repo, tc.Equals, "testing-repo:8080")

	repo, err = podcfg.RecoverRepoFromOperatorPath("docker.io/jujusolutions/jujud-operator:2.6-beta3")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(repo, tc.Equals, "docker.io/jujusolutions")

	_, err = podcfg.RecoverRepoFromOperatorPath("docker.io/jujusolutions/nope:2.6-beta3")
	c.Assert(err, tc.ErrorMatches, `image path "docker.io/jujusolutions/nope:2.6-beta3" does not match the form somerepo/jujud-operator:\.\*`)
}
