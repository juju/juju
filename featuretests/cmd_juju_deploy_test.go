// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"time"

	"github.com/juju/charm/v10"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application/deployer"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testcharms"
)

type cmdDeploySuite struct {
	testing.JujuConnSuite
}

func (s *cmdDeploySuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	// TODO: remove this patch once we removed all the old series from tests in current package.
	s.PatchValue(&deployer.SupportedJujuSeries,
		func(time.Time, string, string) (set.Strings, error) {
			return set.NewStrings(
				"centos7", "centos8", "centos9", "genericlinux", "kubernetes", "opensuseleap",
				"jammy", "focal", "bionic", "xenial",
			), nil
		},
	)
}

func (s *cmdDeploySuite) TestLocalDeploySuccess(c *gc.C) {
	ch := testcharms.Repo.CharmDir("storage-filesystem-subordinate") // has hooks
	ctx, err := runCommand(c, "deploy", ch.Path, "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, `Deploying "storage-filesystem-subordinate" from local charm "storage-filesystem-subordinate", revision 1`)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	savedCh, err := s.State.Charm(charm.MustParseURL("local:bionic/storage-filesystem-subordinate-1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedCh, gc.NotNil)
}

func (s *cmdDeploySuite) TestLocalDeployFailNoHook(c *gc.C) {
	ch := testcharms.Repo.CharmDir("category") // has no hooks
	ctx, err := runCommand(c, "deploy", ch.Path, "--series", "bionic")
	c.Assert(err, gc.NotNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, `invalid charm "category": has no hooks`)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	_, err = s.State.Charm(charm.MustParseURL("local:bionic/category"))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

// The following LXDProfile feature tests are to ensure that we can deploy a
// charm (subordinate charm) and that it passes the validation stages of
// deployment. These tests don't validate that the charm was successfully stood
// up once deployed.

func (s *cmdDeploySuite) TestLocalDeployLXDProfileSuccess(c *gc.C) {
	ch := testcharms.Repo.CharmDir("lxd-profile-subordinate") // has hooks
	ctx, err := runCommand(c, "deploy", ch.Path, "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, `Deploying "lxd-profile-subordinate" from local charm "lxd-profile-subordinate", revision 0`)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	savedCh, err := s.State.Charm(charm.MustParseURL("local:bionic/lxd-profile-subordinate-0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedCh, gc.NotNil)
}

func (s *cmdDeploySuite) TestLocalDeployLXDProfileWithBadConfigSuccess(c *gc.C) {
	ch := testcharms.Repo.CharmDir("lxd-profile-subordinate-fail") // has hooks
	ctx, err := runCommand(c, "deploy", ch.Path, "--series", "bionic")
	c.Assert(err, gc.ErrorMatches, "cmd: error out silently")
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, `ERROR invalid lxd-profile.yaml: contains device type "unix-disk"`)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}
