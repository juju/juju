// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/system"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type ForceDestroySuite struct {
	DestroySuite
}

var _ = gc.Suite(&ForceDestroySuite{})

func (s *ForceDestroySuite) runForceDestroyCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := system.NewForceDestroyCommand(s.api, s.clientapi, s.apierror)
	return testing.RunCommand(c, cmd, args...)
}

func (s *ForceDestroySuite) newForceDestroyCommand() *system.ForceDestroyCommand {
	return system.NewForceDestroyCommand(s.api, s.clientapi, s.apierror)
}

func (s *ForceDestroySuite) TestForceDestroyNoSystemNameError(c *gc.C) {
	_, err := s.runForceDestroyCommand(c)
	c.Assert(err, gc.ErrorMatches, "no system specified")
}

func (s *ForceDestroySuite) TestForceDestroyBadFlags(c *gc.C) {
	_, err := s.runForceDestroyCommand(c, "-n")
	c.Assert(err, gc.ErrorMatches, "flag provided but not defined: -n")
}

func (s *ForceDestroySuite) TestForceDestroyUnknownArgument(c *gc.C) {
	_, err := s.runForceDestroyCommand(c, "environment", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *ForceDestroySuite) TestForceDestroyUnknownSystem(c *gc.C) {
	_, err := s.runForceDestroyCommand(c, "foo")
	c.Assert(err, gc.ErrorMatches, `cannot read system info: environment "foo" not found`)
}

func (s *ForceDestroySuite) TestForceDestroyNonSystemEnvFails(c *gc.C) {
	_, err := s.runForceDestroyCommand(c, "test2")
	c.Assert(err, gc.ErrorMatches, "\"test2\" is not a system; use juju environment destroy to destroy it")
}

func (s *ForceDestroySuite) TestForceDestroySystemNotFoundRemovedFromStore(c *gc.C) {
	s.apierror = errors.NotFoundf("test1")
	ctx, err := s.runForceDestroyCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "system not found, removing config file\n")
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *ForceDestroySuite) TestForceDestroyCannotConnectToAPI(c *gc.C) {
	s.apierror = errors.New("connection refused")
	_, err := s.runForceDestroyCommand(c, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy system: connection refused")
	c.Check(c.GetTestLog(), jc.Contains, "If the system is unusable")
	checkSystemExistsInStore(c, "test1", s.store)
}

func (s *ForceDestroySuite) TestForceDestroy(c *gc.C) {
	_, err := s.runForceDestroyCommand(c, "test1", "-y", "--kill-all-envs")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.envUUID, gc.Equals, "test1-uuid")
	c.Assert(s.api.killAll, jc.IsTrue)
	c.Assert(s.api.ignoreBlock, jc.IsFalse)
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *ForceDestroySuite) TestForceDestroyUnexpectedAPIError(c *gc.C) {
	s.apierror = errors.New("upgrade in progress")
	_, err := s.runForceDestroyCommand(c, "test1", "-y", "--broken", "--kill-all-envs", "--ignore-blocks")
	c.Assert(err, gc.ErrorMatches, "cannot destroy system: upgrade in progress")
	checkSystemExistsInStore(c, "test1", s.store)
}

func (s *ForceDestroySuite) TestForceDestroyEnvironmentGetFails(c *gc.C) {
	s.api.err = errors.NotFoundf(`system "test3"`)
	_, err := s.runForceDestroyCommand(c, "test3", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot obtain bootstrap information: system \"test3\" not found")
	checkSystemExistsInStore(c, "test3", s.store)
}

func (s *ForceDestroySuite) TestForceDestroyFallsBackToClient(c *gc.C) {
	s.api.err = &params.Error{"DestroySystem", params.CodeNotImplemented}
	_, err := s.runForceDestroyCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.clientapi.destroycalled, jc.IsTrue)
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *ForceDestroySuite) TestEnvironmentGetFallsBackToClient(c *gc.C) {
	s.api.err = &params.Error{"EnvironmentGet", params.CodeNotImplemented}
	s.clientapi.env = createBootstrapInfo(c, "test3")
	_, err := s.runForceDestroyCommand(c, "test3", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.clientapi.envgetcalled, jc.IsTrue)
	c.Assert(s.clientapi.destroycalled, jc.IsTrue)
	checkSystemRemovedFromStore(c, "test3", s.store)
}

// TODO (cherylj) Add unauthorized test for falling back to client
func (s *ForceDestroySuite) TestForceDestroyEnvironmentUnauthorized(c *gc.C) {
	s.api.err = common.ErrPerm
	_, err := s.runForceDestroyCommand(c, "test1", "-y", "--broken", "--kill-all-envs", "--ignore-blocks")
	c.Assert(err, gc.ErrorMatches, "cannot destroy system: permission denied")
	c.Assert(s.api.envUUID, gc.Equals, "test1-uuid")
	checkSystemExistsInStore(c, "test1", s.store)
}

func (s *ForceDestroySuite) TestForceDestroyGetsBootstrapInfoFromAPI(c *gc.C) {
	s.api.env = createBootstrapInfo(c, "test3")
	_, err := s.runForceDestroyCommand(c, "test3", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.envUUID, gc.Equals, "test3-uuid")
	checkSystemRemovedFromStore(c, "test3", s.store)
}

func (s *ForceDestroySuite) TestForceDestroyCallsAPIEvenIfBrokenSpecified(c *gc.C) {
	_, err := s.runForceDestroyCommand(c, "test1", "-y", "--broken", "--ignore-blocks")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.envUUID, gc.Equals, "test1-uuid")
	c.Assert(s.api.killAll, jc.IsFalse)
	c.Assert(s.api.ignoreBlock, jc.IsTrue)
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *ForceDestroySuite) TestForceDestroyAPIFailDoesntCallAPI(c *gc.C) {
	s.apierror = system.ErrConnTimedOut
	_, err := s.runForceDestroyCommand(c, "test1", "-y", "--broken")
	c.Assert(s.api.envUUID, gc.Equals, "")
	c.Assert(err, jc.ErrorIsNil)
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *ForceDestroySuite) TestForceDestroyAPIErrorWithForce(c *gc.C) {
	s.api.err = errors.New("something happened")
	_, err := s.runForceDestroyCommand(c, "test1", "-y", "--kill-all-envs", "--ignore-blocks")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.envUUID, gc.Equals, "test1-uuid")
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *ForceDestroySuite) TestForceDestroyReturnsBlocks(c *gc.C) {
	s.api.err = common.ErrOperationBlocked("found blocks in system environments")
	s.api.blocks = []params.EnvironmentBlockInfo{
		params.EnvironmentBlockInfo{
			params.Environment{
				Name:       "test1",
				UUID:       "test1-uuid",
				OwnerTag:   "cheryl@local",
				ServerUUID: "test1-uuid",
			},
			[]string{
				"BlockDestroy",
			},
		},
		params.EnvironmentBlockInfo{
			params.Environment{
				Name:       "test2",
				UUID:       "test2-uuid",
				OwnerTag:   "bob@local",
				ServerUUID: "test1-uuid",
			},
			[]string{
				"BlockDestroy",
				"BlockChange",
			},
		},
	}
	ctx, err := s.runForceDestroyCommand(c, "test1", "-y", "--kill-all-envs")
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
	c.Assert(testing.Stdout(ctx), gc.Equals, `
Unable to destroy system.  Found blocks on the following environment(s):
NAME   ENVIRONMENT UUID  OWNER         BLOCKS
test1  test1-uuid        cheryl@local  destroy-environment
test2  test2-uuid        bob@local     destroy-environment,all-changes

To ignore blocks when destroying the system, run

    juju system force-destroy --ignore-blocks

to remove all blocks in the system before destruction.
`)
}
