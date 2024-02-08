// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"bytes"
	"time"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	jt "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

// Test that the expected methods are called during unregister:
// ControllerByName and RemoveController.
type fakeStore struct {
	jujuclient.ClientStore
	lookupName  string
	removedName string
}

func (s *fakeStore) ControllerByName(name string) (*jujuclient.ControllerDetails, error) {
	s.lookupName = name
	if name != "fake1" {
		return nil, errors.NotFoundf("controller %s", name)
	}
	return &jujuclient.ControllerDetails{}, nil
}

func (s *fakeStore) RemoveController(name string) error {
	// Removing a controller that doesn't exist also returns nil,
	// so no need to check.
	s.removedName = name
	return nil
}

type UnregisterSuite struct {
	jt.IsolationSuite
	store *fakeStore
}

var _ = gc.Suite(&UnregisterSuite{})

func (s *UnregisterSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.store = &fakeStore{}
}

func (s *UnregisterSuite) TestInit(c *gc.C) {
	unregisterCommand := controller.NewUnregisterCommand(s.store)

	err := cmdtesting.InitCommand(unregisterCommand, []string{})
	c.Assert(err, gc.ErrorMatches, "controller name must be specified")

	err = cmdtesting.InitCommand(unregisterCommand, []string{"foo", "bar"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *UnregisterSuite) TestUnregisterUnknownController(c *gc.C) {
	command := controller.NewUnregisterCommand(s.store)
	_, err := cmdtesting.RunCommand(c, command, "fake3")

	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(err, gc.ErrorMatches, "controller fake3 not found")
	c.Check(s.store.lookupName, gc.Equals, "fake3")
}

func (s *UnregisterSuite) TestUnregisterController(c *gc.C) {
	command := controller.NewUnregisterCommand(s.store)
	_, err := cmdtesting.RunCommand(c, command, "fake1", "--no-prompt")

	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.store.lookupName, gc.Equals, "fake1")
	c.Check(s.store.removedName, gc.Equals, "fake1")
}

var unregisterMsg = `
This command will remove connection information for controller "fake1".
Doing so will prevent you from accessing this controller until
you register it again.

To continue, enter the name of the controller to be destroyed: `[1:]

func (s *UnregisterSuite) unregisterCommandAborts(c *gc.C, answer string) {
	var stdin, stderr bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stderr = &stderr
	ctx.Stdin = &stdin

	// Ensure confirmation is requested if "-y" is not specified.
	stdin.WriteString(answer)
	errc := cmdtesting.RunCommandWithContext(ctx, controller.NewUnregisterCommand(s.store), "fake1")
	select {
	case err, ok := <-errc:
		c.Assert(ok, jc.IsTrue)
		c.Check(err, gc.ErrorMatches, "unregistering controller: aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, unregisterMsg)
	c.Check(s.store.lookupName, gc.Equals, "fake1")
	c.Check(s.store.removedName, gc.Equals, "")
}

func (s *UnregisterSuite) TestUnregisterCommandAbortsOnN(c *gc.C) {
	s.unregisterCommandAborts(c, "n")
}

func (s *UnregisterSuite) TestUnregisterCommandAbortsOnNotY(c *gc.C) {
	s.unregisterCommandAborts(c, "foo")
}

func (s *UnregisterSuite) unregisterCommandConfirms(c *gc.C, answer string) {
	var stdin, stdout bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin

	stdin.Reset()
	stdout.Reset()
	stdin.WriteString(answer)
	errc := cmdtesting.RunCommandWithContext(ctx, controller.NewUnregisterCommand(s.store), "fake1")
	select {
	case err, ok := <-errc:
		c.Assert(ok, jc.IsTrue)
		c.Check(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(s.store.lookupName, gc.Equals, "fake1")
	c.Check(s.store.removedName, gc.Equals, "fake1")
}

func (s *UnregisterSuite) TestUnregisterCommandConfirmsOnY(c *gc.C) {
	s.unregisterCommandConfirms(c, "fake1")
}
