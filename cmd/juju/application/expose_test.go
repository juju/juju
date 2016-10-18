// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type ExposeSuite struct {
	jujutesting.RepoSuite
	testing.CmdBlockHelper
}

func (s *ExposeSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = testing.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&ExposeSuite{})

func runExpose(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, NewExposeCommand(), args...)
	return err
}

func (s *ExposeSuite) assertExposed(c *gc.C, application string) {
	svc, err := s.State.Application(application)
	c.Assert(err, jc.ErrorIsNil)
	exposed := svc.IsExposed()
	c.Assert(exposed, jc.IsTrue)
}

func (s *ExposeSuite) TestExpose(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	err := runDeploy(c, ch, "some-application-name", "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "some-application-name", curl, 1, 0)

	err = runExpose(c, "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	s.assertExposed(c, "some-application-name")

	err = runExpose(c, "nonexistent-application")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `application "nonexistent-application" not found`,
		Code:    "not found",
	})
}

func (s *ExposeSuite) TestBlockExpose(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	err := runDeploy(c, ch, "some-application-name", "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "some-application-name", curl, 1, 0)

	// Block operation
	s.BlockAllChanges(c, "TestBlockExpose")

	err = runExpose(c, "some-application-name")
	s.AssertBlocked(c, err, ".*TestBlockExpose.*")
}
