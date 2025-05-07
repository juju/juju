// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"bytes"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jt "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
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

var _ = tc.Suite(&UnregisterSuite{})

func (s *UnregisterSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.store = &fakeStore{}
}

func (s *UnregisterSuite) TestInit(c *tc.C) {
	unregisterCommand := controller.NewUnregisterCommand(s.store)

	err := cmdtesting.InitCommand(unregisterCommand, []string{})
	c.Assert(err, tc.ErrorMatches, "controller name must be specified")

	err = cmdtesting.InitCommand(unregisterCommand, []string{"foo", "bar"})
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *UnregisterSuite) TestUnregisterUnknownController(c *tc.C) {
	command := controller.NewUnregisterCommand(s.store)
	_, err := cmdtesting.RunCommand(c, command, "fake3")

	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(err, tc.ErrorMatches, "controller fake3 not found")
	c.Check(s.store.lookupName, tc.Equals, "fake3")
}

func (s *UnregisterSuite) TestUnregisterController(c *tc.C) {
	command := controller.NewUnregisterCommand(s.store)
	_, err := cmdtesting.RunCommand(c, command, "fake1", "--no-prompt")

	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.store.lookupName, tc.Equals, "fake1")
	c.Check(s.store.removedName, tc.Equals, "fake1")
}

var unregisterMsg = `
This command will remove connection information for controller "fake1".
The controller will not be destroyed but you must register it again
if you want to access it.

To continue, enter the name of the controller to be unregistered: `[1:]

func (s *UnregisterSuite) unregisterCommandAborts(c *tc.C, answer string) {
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
		c.Check(err, tc.ErrorMatches, "unregistering controller: aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, unregisterMsg)
	c.Check(s.store.lookupName, tc.Equals, "fake1")
	c.Check(s.store.removedName, tc.Equals, "")
}

func (s *UnregisterSuite) TestUnregisterCommandAbortsOnN(c *tc.C) {
	s.unregisterCommandAborts(c, "n")
}

func (s *UnregisterSuite) TestUnregisterCommandAbortsOnNotY(c *tc.C) {
	s.unregisterCommandAborts(c, "foo")
}

func (s *UnregisterSuite) unregisterCommandConfirms(c *tc.C, answer string) {
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
	c.Check(s.store.lookupName, tc.Equals, "fake1")
	c.Check(s.store.removedName, tc.Equals, "fake1")
}

func (s *UnregisterSuite) TestUnregisterCommandConfirmsOnY(c *tc.C) {
	s.unregisterCommandConfirms(c, "fake1")
}
