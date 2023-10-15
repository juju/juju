// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
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
	_, err := cmdtesting.RunCommand(c, NewExposeCommand(), args...)
	return err
}

func (s *ExposeSuite) assertExposed(c *gc.C, application string) {
	svc, err := s.State.Application(application)
	c.Assert(err, jc.ErrorIsNil)
	exposed := svc.IsExposed()
	c.Assert(exposed, jc.IsTrue)
}

func (s *ExposeSuite) TestExpose(c *gc.C) {
	s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "some-application-name"})

	err := runExpose(c, "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	s.assertExposed(c, "some-application-name")

	err = runExpose(c, "nonexistent-application")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `application "nonexistent-application" not found`,
		Code:    "not found",
	})
}

func (s *ExposeSuite) TestBlockExpose(c *gc.C) {
	s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "some-application-name"})

	// Block operation
	s.BlockAllChanges(c, "TestBlockExpose")

	err := runExpose(c, "some-application-name")
	s.AssertBlocked(c, err, ".*TestBlockExpose.*")
}
