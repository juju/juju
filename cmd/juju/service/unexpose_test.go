// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/cmd/juju/common"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type UnexposeSuite struct {
	jujutesting.RepoSuite
	common.CmdBlockHelper
}

func (s *UnexposeSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = common.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&UnexposeSuite{})

func runUnexpose(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, NewUnexposeCommand(), args...)
	return err
}

func (s *UnexposeSuite) assertExposed(c *gc.C, service string, expected bool) {
	svc, err := s.State.Service(service)
	c.Assert(err, jc.ErrorIsNil)
	actual := svc.IsExposed()
	c.Assert(actual, gc.Equals, expected)
}

func (s *UnexposeSuite) TestUnexpose(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)

	err = runExpose(c, "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	s.assertExposed(c, "some-service-name", true)

	err = runUnexpose(c, "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	s.assertExposed(c, "some-service-name", false)

	err = runUnexpose(c, "nonexistent-service")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `service "nonexistent-service" not found`,
		Code:    "not found",
	})
}

func (s *UnexposeSuite) TestBlockUnexpose(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)

	// Block operation
	s.BlockAllChanges(c, "TestBlockUnexpose")
	err = runExpose(c, "some-service-name")
	s.AssertBlocked(c, err, ".*TestBlockUnexpose.*")
}
