// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type removeBlocksSuite struct {
	baseControllerSuite
	api   *fakeRemoveBlocksAPI
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&removeBlocksSuite{})

func (s *removeBlocksSuite) SetUpTest(c *gc.C) {
	s.baseControllerSuite.SetUpTest(c)

	err := modelcmd.WriteCurrentController("fake")
	c.Assert(err, jc.ErrorIsNil)

	s.api = &fakeRemoveBlocksAPI{}
	s.store = jujuclienttesting.NewMemStore()
	s.store.Controllers["fake"] = jujuclient.ControllerDetails{}
}

func (s *removeBlocksSuite) newCommand() cmd.Command {
	return controller.NewRemoveBlocksCommandForTest(s.api, s.store)
}

func (s *removeBlocksSuite) TestRemove(c *gc.C) {
	_, err := testing.RunCommand(c, s.newCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.called, jc.IsTrue)
}

func (s *removeBlocksSuite) TestUnrecognizedArg(c *gc.C) {
	_, err := testing.RunCommand(c, s.newCommand(), "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
	c.Assert(s.api.called, jc.IsFalse)
}

func (s *removeBlocksSuite) TestEnvironmentsError(c *gc.C) {
	s.api.err = common.ErrPerm
	_, err := testing.RunCommand(c, s.newCommand())
	c.Assert(err, gc.ErrorMatches, "cannot remove blocks: permission denied")
}

type fakeRemoveBlocksAPI struct {
	err    error
	called bool
}

func (f *fakeRemoveBlocksAPI) Close() error {
	return nil
}

func (f *fakeRemoveBlocksAPI) RemoveBlocks() error {
	f.called = true
	return f.err
}
