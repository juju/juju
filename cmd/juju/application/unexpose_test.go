// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type UnexposeSuite struct {
	jujutesting.RepoSuite
	testing.CmdBlockHelper
}

func (s *UnexposeSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.PatchValue(&supportedJujuSeries, func(time.Time, string, string) (set.Strings, error) {
		return defaultSupportedJujuSeries, nil
	})
	s.CmdBlockHelper = testing.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&UnexposeSuite{})

func runUnexpose(c *gc.C, args ...string) error {
	_, err := cmdtesting.RunCommand(c, NewUnexposeCommand(), args...)
	return err
}

func (s *UnexposeSuite) assertExposed(c *gc.C, application string, expected bool) {
	svc, err := s.State.Application(application)
	c.Assert(err, jc.ErrorIsNil)
	actual := svc.IsExposed()
	c.Assert(actual, gc.Equals, expected)
}

func (s *UnexposeSuite) TestUnexpose(c *gc.C) {
	ch := testcharms.RepoWithSeries("bionic").CharmArchivePath(c.MkDir(), "multi-series")
	err := runDeploy(c, ch, "some-application-name", "--series", "trusty")

	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	s.AssertApplication(c, "some-application-name", curl, 1, 0)

	err = runExpose(c, "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	s.assertExposed(c, "some-application-name", true)

	err = runUnexpose(c, "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	s.assertExposed(c, "some-application-name", false)

	err = runUnexpose(c, "nonexistent-application")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `application "nonexistent-application" not found`,
		Code:    "not found",
	})
}

func (s *UnexposeSuite) TestBlockUnexpose(c *gc.C) {
	ch := testcharms.RepoWithSeries("bionic").CharmArchivePath(c.MkDir(), "multi-series")
	err := runDeploy(c, ch, "some-application-name", "--series", "trusty")

	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	s.AssertApplication(c, "some-application-name", curl, 1, 0)

	// Block operation
	s.BlockAllChanges(c, "TestBlockUnexpose")
	err = runExpose(c, "some-application-name")
	s.AssertBlocked(c, err, ".*TestBlockUnexpose.*")
}
