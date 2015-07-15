// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system_test

import (
	"bytes"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/system"
	cmdtesting "github.com/juju/juju/cmd/testing"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type KillSuite struct {
	DestroySuite
}

var _ = gc.Suite(&KillSuite{})

func (s *KillSuite) SetUpTest(c *gc.C) {
	s.DestroySuite.SetUpTest(c)
}

func (s *KillSuite) runKillCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := system.NewKillCommand(s.api, s.clientapi, s.apierror)
	return testing.RunCommand(c, cmd, args...)
}

func (s *KillSuite) newKillCommand() *system.KillCommand {
	return system.NewKillCommand(s.api, s.clientapi, s.apierror)
}

func (s *KillSuite) TestKillNoSystemNameError(c *gc.C) {
	_, err := s.runKillCommand(c)
	c.Assert(err, gc.ErrorMatches, "no system specified")
}

func (s *KillSuite) TestKillBadFlags(c *gc.C) {
	_, err := s.runKillCommand(c, "-n")
	c.Assert(err, gc.ErrorMatches, "flag provided but not defined: -n")
}

func (s *KillSuite) TestKillUnknownArgument(c *gc.C) {
	_, err := s.runKillCommand(c, "environment", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *KillSuite) TestKillUnknownSystem(c *gc.C) {
	_, err := s.runKillCommand(c, "foo")
	c.Assert(err, gc.ErrorMatches, `cannot read system info: environment "foo" not found`)
}

func (s *KillSuite) TestKillNonSystemEnvFails(c *gc.C) {
	_, err := s.runKillCommand(c, "test2")
	c.Assert(err, gc.ErrorMatches, "\"test2\" is not a system; use juju environment destroy to destroy it")
}

func (s *KillSuite) TestDestroySystemNotFoundNotRemovedFromStore(c *gc.C) {
	s.apierror = errors.NotFoundf("test1")
	_, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy system: test1 not found")
	checkSystemExistsInStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillCannotConnectToAPISucceeds(c *gc.C) {
	s.apierror = errors.New("connection refused")
	_, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(c.GetTestLog(), jc.Contains, "Unable to open API: connection refused")
	checkSystemRemovedFromStore(c, "test1", s.store)

	// Check that we didn't call the API
	c.Assert(s.api.ignoreBlocks, jc.IsFalse)
}

func (s *KillSuite) TestKillWithAPIConnection(c *gc.C) {
	_, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.ignoreBlocks, jc.IsTrue)
	c.Assert(s.api.destroyAll, jc.IsTrue)
	c.Assert(s.clientapi.destroycalled, jc.IsFalse)
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillEnvironmentGetFailsWithoutAPIConnection(c *gc.C) {
	s.apierror = errors.New("connection refused")
	s.api.err = errors.NotFoundf(`system "test3"`)
	_, err := s.runKillCommand(c, "test3", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot obtain bootstrap information: unable to get bootstrap information from API")
	checkSystemExistsInStore(c, "test3", s.store)
}

func (s *KillSuite) TestKillEnvironmentGetFailsWithAPIConnection(c *gc.C) {
	s.api.err = errors.NotFoundf(`system "test3"`)
	_, err := s.runKillCommand(c, "test3", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot obtain bootstrap information: system \"test3\" not found")
	checkSystemExistsInStore(c, "test3", s.store)
}

func (s *KillSuite) TestKillFallsBackToClient(c *gc.C) {
	s.api.err = &params.Error{"DestroySystem", params.CodeNotImplemented}
	_, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.clientapi.destroycalled, jc.IsTrue)
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestClientKillDestroysSystemWithAPIError(c *gc.C) {
	s.api.err = &params.Error{"DestroySystem", params.CodeNotImplemented}
	s.clientapi.err = errors.New("some destroy error")
	_, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(c.GetTestLog(), jc.Contains, "Unable to destroy system through the API: some destroy error.  Destroying through provider.")
	c.Assert(s.clientapi.destroycalled, jc.IsTrue)
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillDestroysSystemWithAPIError(c *gc.C) {
	s.api.err = errors.New("some destroy error")
	_, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(c.GetTestLog(), jc.Contains, "Unable to destroy system through the API: some destroy error.  Destroying through provider.")
	c.Assert(s.api.ignoreBlocks, jc.IsTrue)
	c.Assert(s.api.destroyAll, jc.IsTrue)
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillCommandConfirmation(c *gc.C) {
	var stdin, stdout bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin

	// Ensure confirmation is requested if "-y" is not specified.
	stdin.WriteString("n")
	_, errc := cmdtesting.RunCommand(ctx, s.newKillCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "system destruction aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(testing.Stdout(ctx), gc.Matches, "WARNING!.*test1(.|\n)*")
	checkSystemExistsInStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillAPIPermErrFails(c *gc.C) {
	testDialer := func(sysName string) (*api.State, error) {
		return nil, common.ErrPerm
	}
	s.PatchValue(&system.DialAPI, testDialer)

	cmd := system.NewKillCommand(nil, nil, nil)
	_, err := testing.RunCommand(c, cmd, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy system: permission denied")
	c.Assert(s.api.ignoreBlocks, jc.IsFalse)
	checkSystemExistsInStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillEarlyAPIConnectionTimeout(c *gc.C) {
	testDialer := func(sysName string) (*api.State, error) {
		time.Sleep(5 * time.Minute)
		return nil, errors.New("kill command waited too long")
	}
	s.PatchValue(&system.DialAPI, testDialer)

	cmd := system.NewKillCommand(nil, nil, nil)
	_, err := testing.RunCommand(c, cmd, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(c.GetTestLog(), jc.Contains, "Unable to open API: connection to state server timed out")
	c.Assert(s.api.ignoreBlocks, jc.IsFalse)
	c.Assert(s.api.destroyAll, jc.IsFalse)
	checkSystemRemovedFromStore(c, "test1", s.store)
}
