// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/cmd/envcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type ExposeSuite struct {
	jujutesting.RepoSuite
	CmdBlockHelper
}

func (s *ExposeSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&ExposeSuite{})

func runExpose(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&ExposeCommand{}), args...)
	return err
}

func (s *ExposeSuite) assertExposed(c *gc.C, service string) {
	svc, err := s.State.Service(service)
	c.Assert(err, jc.ErrorIsNil)
	exposed := svc.IsExposed()
	c.Assert(exposed, jc.IsTrue)
}

func (s *ExposeSuite) TestExpose(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)

	err = runExpose(c, "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	s.assertExposed(c, "some-service-name")

	err = runExpose(c, "nonexistent-service")
	c.Assert(err, gc.ErrorMatches, `service "nonexistent-service" not found`)
}

func (s *ExposeSuite) TestBlockExpose(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)

	// Block operation
	s.BlockAllChanges(c, "TestBlockExpose")

	err = runExpose(c, "some-service-name")
	s.AssertBlocked(c, err, ".*TestBlockExpose.*")
}
