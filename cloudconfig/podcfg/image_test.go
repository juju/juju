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
	jujuversion "github.com/juju/juju/version"
)

type imageSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&imageSuite{})

func (s *imageSuite) TestGetJujuOCIImagePath(c *gc.C) {
	cfg := testing.FakeControllerConfig()

	cfg[controller.CAASImageRepo] = "testing-repo"
	ver := version.MustParse("2.6-beta3")
	path, err := podcfg.GetJujuOCIImagePath(cfg, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, jc.DeepEquals, "testing-repo/jujud-operator:2.6-beta3")

	cfg[controller.CAASOperatorImagePath] = "testing-old-repo/jujud-old-operator:1.6"
	path, err = podcfg.GetJujuOCIImagePath(cfg, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, jc.DeepEquals, "testing-old-repo/jujud-old-operator:2.6-beta3")
}

func (s *imageSuite) TestGetJujuOCIImagePaths(c *gc.C) {
	cfg := testing.FakeControllerConfig()

	cfg[controller.CAASImageRepo] = "testing-repo"
	ver := version.MustParse("2.6-beta3")
	paths, err := podcfg.GetJujuOCIImagePaths(cfg, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(paths, jc.DeepEquals, []string{"testing-repo/jujud-operator:2.6-beta3"})

	cfg[controller.CAASOperatorImagePath] = "testing-old-repo/jujud-old-operator:1.6"
	paths, err = podcfg.GetJujuOCIImagePaths(cfg, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(paths, jc.DeepEquals, []string{"testing-old-repo/jujud-old-operator:2.6-beta3"})
}

func (s *imageSuite) TestGetJujuOCIGitImagePath(c *gc.C) {
	cfg := testing.FakeControllerConfig()

	ver := version.MustParse("2.6-beta3")
	gitCommit := "ffffffffffffffffffffffffffffffffffffffff"
	s.PatchValue(&jujuversion.Current, ver)
	s.PatchValue(&jujuversion.GitCommit, gitCommit)

	cfg[controller.CAASImageRepo] = "testing-repo"
	paths, err := podcfg.GetJujuOCIImagePaths(cfg, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(paths, jc.DeepEquals, []string{
		"testing-repo/jujud-operator-git:2.6-beta3-ffffffffffffffffffffffffffffffffffffffff",
		"testing-repo/jujud-operator:2.6-beta3"})

	cfg[controller.CAASOperatorImagePath] = "testing-old-repo/jujud-operator:1.6"
	paths, err = podcfg.GetJujuOCIImagePaths(cfg, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(paths, jc.DeepEquals, []string{
		"testing-old-repo/jujud-operator-git:2.6-beta3-ffffffffffffffffffffffffffffffffffffffff",
		"testing-old-repo/jujud-operator:2.6-beta3"})

	cfg[controller.CAASOperatorImagePath] = "testing-old-repo/jujud-old-operator:1.6"
	paths, err = podcfg.GetJujuOCIImagePaths(cfg, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(paths, jc.DeepEquals, []string{"testing-old-repo/jujud-old-operator:2.6-beta3"})
}

func (s *imageSuite) TestGetJujuOCIAlternateRepo(c *gc.C) {
	cfg := testing.FakeControllerConfig()

	ver := version.MustParse("2.6-beta3")
	gitCommit := "ffffffffffffffffffffffffffffffffffffffff"
	s.PatchValue(&jujuversion.Current, ver)
	s.PatchValue(&jujuversion.GitCommit, gitCommit)

	cfg[controller.CAASImageRepo] = "gcr.io/some-project-123acf"
	paths, err := podcfg.GetJujuOCIImagePaths(cfg, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(paths, jc.DeepEquals, []string{
		"gcr.io/some-project-123acf/jujud-operator-git:2.6-beta3-ffffffffffffffffffffffffffffffffffffffff",
		"gcr.io/some-project-123acf/jujud-operator:2.6-beta3"})

	cfg[controller.CAASOperatorImagePath] = "gcr.io/some-project-foobar/jujud-operator:1.6"
	paths, err = podcfg.GetJujuOCIImagePaths(cfg, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(paths, jc.DeepEquals, []string{
		"gcr.io/some-project-foobar/jujud-operator-git:2.6-beta3-ffffffffffffffffffffffffffffffffffffffff",
		"gcr.io/some-project-foobar/jujud-operator:2.6-beta3"})

	cfg[controller.CAASOperatorImagePath] = "gcr.io/some-project-badfood/jujud-old-operator:1.6"
	paths, err = podcfg.GetJujuOCIImagePaths(cfg, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(paths, jc.DeepEquals, []string{"gcr.io/some-project-badfood/jujud-old-operator:2.6-beta3"})
}

func (s *imageSuite) TestGetJujuOCIHashRefImage(c *gc.C) {
	cfg := testing.FakeControllerConfig()

	ver := version.MustParse("2.6-beta3")
	gitCommit := "ffffffffffffffffffffffffffffffffffffffff"
	s.PatchValue(&jujuversion.Current, ver)
	s.PatchValue(&jujuversion.GitCommit, gitCommit)

	cfg[controller.CAASOperatorImagePath] = "jujud-what@sha256:0000000000000000000000000000000000000000000000000000000000000000"
	paths, err := podcfg.GetJujuOCIImagePaths(cfg, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(paths, jc.DeepEquals, []string{"jujud-what:2.6-beta3"})
}

func (s *imageSuite) TestIsJujuOCIImage(c *gc.C) {
	c.Assert(podcfg.IsJujuOCIImage("jujuju:2.6.5"), jc.IsFalse)
	c.Assert(podcfg.IsJujuOCIImage("jujusolutions/jujud-operatord:2.6.5"), jc.IsFalse)
	c.Assert(podcfg.IsJujuOCIImage("jujusolutions/jujud-operator:2.6.5"), jc.IsTrue)
	c.Assert(podcfg.IsJujuOCIImage("jujusolutions/jujud-operator-git:2.6.5-ffffffffffffffffffffffffffffffffffffffff"), jc.IsTrue)
	c.Assert(podcfg.IsJujuOCIImage("jujusolutions/jujud-operator"), jc.IsTrue)
	c.Assert(podcfg.IsJujuOCIImage("jujusolutions/jujud-operator-git"), jc.IsTrue)
	c.Assert(podcfg.IsJujuOCIImage("gcr.io/jujusolutions/jujud-operatord:2.6.5"), jc.IsFalse)
	c.Assert(podcfg.IsJujuOCIImage("gcr.io/jujusolutions/jujud-operator:2.6.5"), jc.IsTrue)
	c.Assert(podcfg.IsJujuOCIImage("gcr.io/jujusolutions/jujud-operator-git:2.6.5-ffffffffffffffffffffffffffffffffffffffff"), jc.IsTrue)
	c.Assert(podcfg.IsJujuOCIImage("gcr.io:443/jujusolutions/jujud-operatord:2.6.5"), jc.IsFalse)
	c.Assert(podcfg.IsJujuOCIImage("gcr.io:443/jujusolutions/jujud-operator:2.6.5"), jc.IsTrue)
	c.Assert(podcfg.IsJujuOCIImage("gcr.io:443/jujusolutions/jujud-operator-git:2.6.5-ffffffffffffffffffffffffffffffffffffffff"), jc.IsTrue)
	c.Assert(podcfg.IsJujuOCIImage("jujud-operator"), jc.IsTrue)
	c.Assert(podcfg.IsJujuOCIImage("jujud-operator-git"), jc.IsTrue)
	c.Assert(podcfg.IsJujuOCIImage("juju-db"), jc.IsFalse)
	c.Assert(podcfg.IsJujuOCIImage("jujusolutions/juju-db"), jc.IsFalse)
	c.Assert(podcfg.IsJujuOCIImage("jujusolutions/jujud-operator@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsTrue)
	c.Assert(podcfg.IsJujuOCIImage("jujusolutions/jujud-operator:2.6.5@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsTrue)
	c.Assert(podcfg.IsJujuOCIImage("gcr.io/jujusolutions/juju-db"), jc.IsFalse)
	c.Assert(podcfg.IsJujuOCIImage("gcr.io/jujusolutions/jujud-operator@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsTrue)
	c.Assert(podcfg.IsJujuOCIImage("gcr.io/jujusolutions/jujud-operator:2.6.5@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsTrue)
}

func (s *imageSuite) TestIsJujuDBOCIImage(c *gc.C) {
	c.Assert(podcfg.IsJujuDBOCIImage("juju-db"), jc.IsTrue)
	c.Assert(podcfg.IsJujuDBOCIImage("jujusolutions/juju-db"), jc.IsTrue)
	c.Assert(podcfg.IsJujuDBOCIImage("jujusolutions/juju-db:4.0"), jc.IsTrue)
	c.Assert(podcfg.IsJujuDBOCIImage("jujusolutions/juju-db@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsTrue)
	c.Assert(podcfg.IsJujuDBOCIImage("jujusolutions/juju-db:4.0@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsTrue)
	c.Assert(podcfg.IsJujuDBOCIImage("gcr.io/jujusolutions/juju-db"), jc.IsTrue)
	c.Assert(podcfg.IsJujuDBOCIImage("gcr.io/jujusolutions/juju-db:4.0"), jc.IsTrue)
	c.Assert(podcfg.IsJujuDBOCIImage("gcr.io/jujusolutions/juju-db@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsTrue)
	c.Assert(podcfg.IsJujuDBOCIImage("gcr.io/jujusolutions/juju-db:4.0@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsTrue)
	c.Assert(podcfg.IsJujuDBOCIImage("gcr.io/jujusolutions/juju-db"), jc.IsTrue)
	c.Assert(podcfg.IsJujuDBOCIImage("gcr.io:443/jujusolutions/juju-db:4.0"), jc.IsTrue)
	c.Assert(podcfg.IsJujuDBOCIImage("gcr.io:443/jujusolutions/juju-db@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsTrue)
	c.Assert(podcfg.IsJujuDBOCIImage("gcr.io:443/jujusolutions/juju-db:4.0@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsTrue)
	c.Assert(podcfg.IsJujuDBOCIImage("jujuju:2.6.5"), jc.IsFalse)
	c.Assert(podcfg.IsJujuDBOCIImage("jujusolutions/jujud-operatord:2.6.5"), jc.IsFalse)
	c.Assert(podcfg.IsJujuDBOCIImage("jujusolutions/jujud-operator:2.6.5"), jc.IsFalse)
	c.Assert(podcfg.IsJujuDBOCIImage("jujusolutions/jujud-operator-git:2.6.5-ffffffffffffffffffffffffffffffffffffffff"), jc.IsFalse)
	c.Assert(podcfg.IsJujuDBOCIImage("jujusolutions/jujud-operator"), jc.IsFalse)
	c.Assert(podcfg.IsJujuDBOCIImage("jujusolutions/jujud-operator-git"), jc.IsFalse)
	c.Assert(podcfg.IsJujuDBOCIImage("jujud-operator"), jc.IsFalse)
	c.Assert(podcfg.IsJujuDBOCIImage("jujud-operator-git"), jc.IsFalse)
	c.Assert(podcfg.IsJujuDBOCIImage("jujusolutions/jujud-operator@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsFalse)
	c.Assert(podcfg.IsJujuDBOCIImage("jujusolutions/jujud-operator:2.6.5@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsFalse)
	c.Assert(podcfg.IsJujuDBOCIImage("gcr.io/jujusolutions/jujud-operator@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsFalse)
	c.Assert(podcfg.IsJujuDBOCIImage("gcr.io/jujusolutions/jujud-operator:2.6.5@sha256:0000000000000000000000000000000000000000000000000000000000000000"), jc.IsFalse)
}

func (s *imageSuite) TestNormaliseImagePath(c *gc.C) {
	must := func(imagePath string) string {
		res, err := podcfg.NormaliseImagePath(imagePath)
		c.Assert(err, jc.ErrorIsNil)
		return res
	}
	c.Assert(must("jujuju:2.6.5"),
		gc.Equals, "jujuju")
	c.Assert(must("jujusolutions/jujud-operatord:2.6.5"),
		gc.Equals, "jujusolutions/jujud-operatord")
	c.Assert(must("jujusolutions/jujud-operator:2.6.5"),
		gc.Equals, "jujusolutions/jujud-operator")
	c.Assert(must("jujusolutions/jujud-operator-git:2.6.5-ffffffffffffffffffffffffffffffffffffffff"),
		gc.Equals, "jujusolutions/jujud-operator-git")
	c.Assert(must("jujusolutions/jujud-operator"),
		gc.Equals, "jujusolutions/jujud-operator")
	c.Assert(must("jujusolutions/jujud-operator-git"),
		gc.Equals, "jujusolutions/jujud-operator-git")
	c.Assert(must("gcr.io/jujusolutions/jujud-operatord:2.6.5"),
		gc.Equals, "gcr.io/jujusolutions/jujud-operatord")
	c.Assert(must("gcr.io/jujusolutions/jujud-operator:2.6.5"),
		gc.Equals, "gcr.io/jujusolutions/jujud-operator")
	c.Assert(must("gcr.io/jujusolutions/jujud-operator-git:2.6.5-ffffffffffffffffffffffffffffffffffffffff"),
		gc.Equals, "gcr.io/jujusolutions/jujud-operator-git")
	c.Assert(must("gcr.io:443/jujusolutions/jujud-operatord:2.6.5"),
		gc.Equals, "gcr.io:443/jujusolutions/jujud-operatord")
	c.Assert(must("gcr.io:443/jujusolutions/jujud-operator:2.6.5"),
		gc.Equals, "gcr.io:443/jujusolutions/jujud-operator")
	c.Assert(must("gcr.io:443/jujusolutions/jujud-operator-git:2.6.5-ffffffffffffffffffffffffffffffffffffffff"),
		gc.Equals, "gcr.io:443/jujusolutions/jujud-operator-git")
	c.Assert(must("jujud-operator"),
		gc.Equals, "jujud-operator")
	c.Assert(must("jujud-operator-git"),
		gc.Equals, "jujud-operator-git")
	c.Assert(must("juju-db"),
		gc.Equals, "juju-db")
	c.Assert(must("jujusolutions/juju-db"),
		gc.Equals, "jujusolutions/juju-db")
	c.Assert(must("jujusolutions/jujud-operator@sha256:0000000000000000000000000000000000000000000000000000000000000000"),
		gc.Equals, "jujusolutions/jujud-operator")
	c.Assert(must("jujusolutions/jujud-operator:2.6.5@sha256:0000000000000000000000000000000000000000000000000000000000000000"),
		gc.Equals, "jujusolutions/jujud-operator")
	c.Assert(must("gcr.io/jujusolutions/juju-db"),
		gc.Equals, "gcr.io/jujusolutions/juju-db")
	c.Assert(must("gcr.io/jujusolutions/jujud-operator@sha256:0000000000000000000000000000000000000000000000000000000000000000"),
		gc.Equals, "gcr.io/jujusolutions/jujud-operator")
	c.Assert(must("gcr.io/jujusolutions/jujud-operator:2.6.5@sha256:0000000000000000000000000000000000000000000000000000000000000000"),
		gc.Equals, "gcr.io/jujusolutions/jujud-operator")
}
