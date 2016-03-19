// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type ControllerCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&ControllerCommandSuite{})

func (s *ControllerCommandSuite) TestControllerCommandNoneSpecified(c *gc.C) {
	_, err := initTestControllerCommand(c, nil)
	c.Assert(err, gc.ErrorMatches, "no controller specified")
}

func (s *ControllerCommandSuite) TestControllerCommandInitSystemFile(c *gc.C) {
	// If there is a current-controller file, use that.
	err := modelcmd.WriteCurrentController("foo")
	c.Assert(err, jc.ErrorIsNil)
	store := jujuclienttesting.NewMemStore()
	store.Accounts["foo"] = &jujuclient.ControllerAccounts{
		CurrentAccount: "bar@baz",
	}
	store.Controllers["foo"] = jujuclient.ControllerDetails{}
	testEnsureControllerName(c, store, "foo")
}

func (s *ControllerCommandSuite) TestControllerCommandInitExplicit(c *gc.C) {
	// Take controller name from command line arg, and it trumps the current-
	// controller file.
	err := modelcmd.WriteCurrentController("foo")
	c.Assert(err, jc.ErrorIsNil)
	store := jujuclienttesting.NewMemStore()
	store.Accounts["explicit"] = &jujuclient.ControllerAccounts{
		CurrentAccount: "bar@baz",
	}
	store.Controllers["explicit"] = jujuclient.ControllerDetails{}
	testEnsureControllerName(c, store, "explicit", "-c", "explicit")
	testEnsureControllerName(c, store, "explicit", "--controller", "explicit")
}

func (s *ControllerCommandSuite) TestWrapWithoutFlags(c *gc.C) {
	cmd := new(testControllerCommand)
	wrapped := modelcmd.WrapController(cmd, modelcmd.ControllerSkipFlags)
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
	panic("should not be called")
}

func initTestControllerCommand(c *gc.C, store jujuclient.ClientStore, args ...string) (*testControllerCommand, error) {
	cmd := new(testControllerCommand)
	cmd.SetClientStore(store)
	wrapped := modelcmd.WrapController(cmd)
	return cmd, cmdtesting.InitCommand(wrapped, args)
}

func testEnsureControllerName(c *gc.C, store jujuclient.ClientStore, expect string, args ...string) {
	cmd, err := initTestControllerCommand(c, store, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.ControllerName(), gc.Equals, expect)
}

type ControllerSuite struct {
	store jujuclient.ControllerStore
}

var _ = gc.Suite(&ControllerSuite{})

func (s *ControllerSuite) SetUpTest(c *gc.C) {
	controller := jujuclient.ControllerDetails{ControllerUUID: "controller-uuid"}
	anothercontroller := jujuclient.ControllerDetails{ControllerUUID: "another-uuid"}
	s.store = &jujuclienttesting.MemStore{
		Controllers: map[string]jujuclient.ControllerDetails{
			"local.controller":        controller,
			"anothercontroller":       anothercontroller,
			"local.anothercontroller": jujuclient.ControllerDetails{},
		},
	}
}

func (s *ControllerSuite) TestLocalNameFound(c *gc.C) {
	name, err := modelcmd.ResolveControllerName(s.store, "local.controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.DeepEquals, "local.controller")
}

func (s *ControllerSuite) TestLocalNameFallback(c *gc.C) {
	name, err := modelcmd.ResolveControllerName(s.store, "controller")
	c.Assert(name, gc.DeepEquals, "local.controller")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestNonLocalController(c *gc.C) {
	name, err := modelcmd.ResolveControllerName(s.store, "anothercontroller")
	c.Assert(name, gc.DeepEquals, "anothercontroller")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestOnlyLocalController(c *gc.C) {
	name, err := modelcmd.ResolveControllerName(s.store, "local.anothercontroller")
	c.Assert(name, gc.DeepEquals, "local.anothercontroller")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestNotFound(c *gc.C) {
	_, err := modelcmd.ResolveControllerName(s.store, "foo")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	// We should report on the passed in controller name.
	c.Assert(err, gc.ErrorMatches, ".* foo .*")
}
