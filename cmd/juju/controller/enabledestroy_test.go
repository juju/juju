// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"

	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/jujuclient"
)

type enableDestroyControllerSuite struct {
	baseControllerSuite
	api   *fakeRemoveBlocksAPI
	store *jujuclient.MemStore
}

var _ = tc.Suite(&enableDestroyControllerSuite{})

func (s *enableDestroyControllerSuite) SetUpTest(c *tc.C) {
	s.baseControllerSuite.SetUpTest(c)

	s.api = &fakeRemoveBlocksAPI{}
	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "fake"
	s.store.Controllers["fake"] = jujuclient.ControllerDetails{}
}

func (s *enableDestroyControllerSuite) newCommand() cmd.Command {
	return controller.NewEnableDestroyControllerCommandForTest(s.api, s.store)
}

func (s *enableDestroyControllerSuite) TestRemove(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.api.called, tc.IsTrue)
}

func (s *enableDestroyControllerSuite) TestUnrecognizedArg(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.newCommand(), "whoops")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["whoops"\]`)
	c.Assert(s.api.called, tc.IsFalse)
}

func (s *enableDestroyControllerSuite) TestEnvironmentsError(c *tc.C) {
	s.api.err = apiservererrors.ErrPerm
	_, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

type fakeRemoveBlocksAPI struct {
	err    error
	called bool
}

func (f *fakeRemoveBlocksAPI) Close() error {
	return nil
}

func (f *fakeRemoveBlocksAPI) RemoveBlocks(ctx context.Context) error {
	f.called = true
	return f.err
}
