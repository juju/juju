// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	utilexec "github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v4"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type FactorySuite struct {
	testing.IsolationSuite
	factory operation.Factory
}

var _ = gc.Suite(&FactorySuite{})

func (s *FactorySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	// Yes, this factory will produce useless ops; this suite is just for
	// verifying that inadequate args to the factory methods will produce
	// the expected errors; and that the results of same get a string
	// representation that does not depend on the factory attributes.
	s.factory = operation.NewFactory(nil, nil, nil, nil)
}

func (s *FactorySuite) TestNewDeploy(c *gc.C) {
	op, err := s.factory.NewInstall(nil)
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "charm url required")

	charmURL := corecharm.MustParseURL("cs:quantal/wordpress-1")
	op, err = s.factory.NewInstall(charmURL)
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "install cs:quantal/wordpress-1")

	op, err = s.factory.NewUpgrade(nil)
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "charm url required")

	op, err = s.factory.NewUpgrade(charmURL)
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "upgrade to cs:quantal/wordpress-1")

	op, err = s.factory.NewRevertUpgrade(nil)
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "charm url required")

	op, err = s.factory.NewRevertUpgrade(charmURL)
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "clear resolved flag and switch upgrade to cs:quantal/wordpress-1")

	op, err = s.factory.NewResolvedUpgrade(nil)
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "charm url required")

	op, err = s.factory.NewResolvedUpgrade(charmURL)
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "clear resolved flag and continue upgrade to cs:quantal/wordpress-1")
}

func (s *FactorySuite) TestNewAction(c *gc.C) {
	op, err := s.factory.NewAction("lol-something")
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `invalid action id "lol-something"`)

	op, err = s.factory.NewAction(someActionId)
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "run action "+someActionId)
}

func (s *FactorySuite) TestNewCommands(c *gc.C) {
	sendResponse := func(*utilexec.ExecResponse, error) {
		panic("don't call this")
	}
	args := func(commands string, relationId int, remoteUnit string) operation.CommandArgs {
		return operation.CommandArgs{
			Commands:       commands,
			RelationId:     relationId,
			RemoteUnitName: remoteUnit,
		}
	}
	op, err := s.factory.NewCommands(args("", -1, ""), sendResponse)
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "commands required")

	op, err = s.factory.NewCommands(args("any old thing", -1, ""), nil)
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "response sender required")

	op, err = s.factory.NewCommands(args("any old thing", -1, "unit/1"), sendResponse)
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "remote unit not valid without relation")

	op, err = s.factory.NewCommands(args("any old thing", 0, "lol"), sendResponse)
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `invalid remote unit name "lol"`)

	op, err = s.factory.NewCommands(args("any old thing", -1, ""), sendResponse)
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "run commands")

	op, err = s.factory.NewCommands(args("any old thing", 1, ""), sendResponse)
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "run commands (1)")

	op, err = s.factory.NewCommands(args("any old thing", 1, "unit/1"), sendResponse)
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "run commands (1; unit/1)")
}

func (s *FactorySuite) TestNewHook(c *gc.C) {
	op, err := s.factory.NewRunHook(hook.Info{Kind: hooks.Kind("gibberish")})
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `unknown hook kind "gibberish"`)

	op, err = s.factory.NewRunHook(hook.Info{Kind: hooks.Install})
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "run install hook")

	op, err = s.factory.NewRetryHook(hook.Info{Kind: hooks.Kind("gibberish")})
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `unknown hook kind "gibberish"`)

	op, err = s.factory.NewRetryHook(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 123,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "clear resolved flag and run relation-broken (123) hook")

	op, err = s.factory.NewSkipHook(hook.Info{Kind: hooks.Kind("gibberish")})
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `unknown hook kind "gibberish"`)

	op, err = s.factory.NewSkipHook(hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "foo/22",
		RelationId: 123,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "clear resolved flag and skip run relation-joined (123; foo/22) hook")
}
