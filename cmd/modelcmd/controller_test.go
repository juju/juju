// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

type ControllerCommandSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ControllerCommandSuite{})

func (s *ControllerCommandSuite) TestControllerCommandNoneSpecified(c *gc.C) {
	_, _, err := initTestControllerCommand(c, nil)
	c.Assert(errors.Cause(err), gc.Equals, modelcmd.ErrNoControllersDefined)
}

func (s *ControllerCommandSuite) TestControllerCommandInitCurrentController(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar",
	}
	store.Controllers["foo"] = jujuclient.ControllerDetails{}
	testEnsureControllerName(c, store, "foo")
}

func (s *ControllerCommandSuite) TestControllerCommandInitExplicit(c *gc.C) {
	// Take controller name from command line arg, and it trumps the current-
	// controller file.
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Accounts["explicit"] = jujuclient.AccountDetails{
		User: "bar",
	}
	store.Controllers["explicit"] = jujuclient.ControllerDetails{}
	testEnsureControllerName(c, store, "explicit", "-c", "explicit")
	testEnsureControllerName(c, store, "explicit", "--controller", "explicit")
}

func (s *ControllerCommandSuite) TestWrapWithoutFlags(c *gc.C) {
	cmd := new(testControllerCommand)
	wrapped := modelcmd.WrapController(cmd, modelcmd.WrapControllerSkipControllerFlags)
	err := cmdtesting.InitCommand(wrapped, []string{"-s", "testsys"})
	c.Assert(err, gc.ErrorMatches, "flag provided but not defined: -s")
}

type testControllerCommand struct {
	modelcmd.ControllerCommandBase
}

func (c *testControllerCommand) Info() *cmd.Info {
	panic("should not be called")
}

func (c *testControllerCommand) Run(ctx *cmd.Context) error {
	return nil
}

func initTestControllerCommand(c *gc.C, store jujuclient.ClientStore, args ...string) (cmd.Command, *testControllerCommand, error) {
	cmd := new(testControllerCommand)
	cmd.SetClientStore(store)
	wrapped := modelcmd.WrapController(cmd)
	if err := cmdtesting.InitCommand(wrapped, args); err != nil {
		return nil, nil, err
	}
	return wrapped, cmd, nil
}

func testEnsureControllerName(c *gc.C, store jujuclient.ClientStore, expect string, args ...string) {
	cmd, controllerCmd, err := initTestControllerCommand(c, store, args...)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerCmd.ControllerName(), gc.Equals, expect)
}
