// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	utilexec "github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"

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
	s.factory = operation.NewFactory(operation.FactoryParams{})
}

func (s *FactorySuite) testNewDeployError(c *gc.C, newDeploy newDeploy) {
	op, err := newDeploy(s.factory, nil)
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "charm url required")
}

func (s *FactorySuite) TestNewInstallError(c *gc.C) {
	s.testNewDeployError(c, (operation.Factory).NewInstall)
}

func (s *FactorySuite) TestNewUpgradeError(c *gc.C) {
	s.testNewDeployError(c, (operation.Factory).NewUpgrade)
}

func (s *FactorySuite) TestNewRevertUpgradeError(c *gc.C) {
	s.testNewDeployError(c, (operation.Factory).NewRevertUpgrade)
}

func (s *FactorySuite) TestNewResolvedUpgradeError(c *gc.C) {
	s.testNewDeployError(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *FactorySuite) testNewDeployString(c *gc.C, newDeploy newDeploy, expectPrefix string) {
	op, err := newDeploy(s.factory, corecharm.MustParseURL("cs:quantal/wordpress-1"))
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, expectPrefix+" cs:quantal/wordpress-1")
}

func (s *FactorySuite) TestNewInstallString(c *gc.C) {
	s.testNewDeployString(c, (operation.Factory).NewInstall, "install")
}

func (s *FactorySuite) TestNewUpgradeString(c *gc.C) {
	s.testNewDeployString(c, (operation.Factory).NewUpgrade, "upgrade to")
}

func (s *FactorySuite) TestNewRevertUpgradeString(c *gc.C) {
	s.testNewDeployString(c,
		(operation.Factory).NewRevertUpgrade,
		"clear resolved flag and switch upgrade to",
	)
}

func (s *FactorySuite) TestNewResolvedUpgradeString(c *gc.C) {
	s.testNewDeployString(c,
		(operation.Factory).NewResolvedUpgrade,
		"clear resolved flag and continue upgrade to",
	)
}

func (s *FactorySuite) TestNewActionError(c *gc.C) {
	op, err := s.factory.NewAction("lol-something")
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `invalid action id "lol-something"`)
}

func (s *FactorySuite) TestNewActionString(c *gc.C) {
	op, err := s.factory.NewAction(someActionId)
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "run action "+someActionId)
}

func panicSendResponse(*utilexec.ExecResponse, error) {
	panic("don't call this")
}

func commandArgs(commands string, relationId int, remoteUnit string) operation.CommandArgs {
	return operation.CommandArgs{
		Commands:       commands,
		RelationId:     relationId,
		RemoteUnitName: remoteUnit,
	}
}

func (s *FactorySuite) TestNewCommandsSendResponseError(c *gc.C) {
	op, err := s.factory.NewCommands(commandArgs("anything", -1, ""), nil)
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "response sender required")
}

func (s *FactorySuite) testNewCommandsArgsError(
	c *gc.C, args operation.CommandArgs, expect string,
) {
	op, err := s.factory.NewCommands(args, panicSendResponse)
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *FactorySuite) TestNewCommandsArgsError_NoCommand(c *gc.C) {
	s.testNewCommandsArgsError(c,
		commandArgs("", -1, ""),
		"commands required",
	)
}

func (s *FactorySuite) TestNewCommandsArgsError_BadRemoteUnit(c *gc.C) {
	s.testNewCommandsArgsError(c,
		commandArgs("any old thing", -1, "unit/1"),
		"remote unit not valid without relation",
	)
}

func (s *FactorySuite) TestNewCommandsArgsError_BadRemoteUnitName(c *gc.C) {
	s.testNewCommandsArgsError(c,
		commandArgs("any old thing", 0, "lol"),
		`invalid remote unit name "lol"`,
	)
}

func (s *FactorySuite) testNewCommandsString(
	c *gc.C, args operation.CommandArgs, expect string,
) {
	op, err := s.factory.NewCommands(args, panicSendResponse)
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, expect)
}

func (s *FactorySuite) TestNewCommandsString_CommandsOnly(c *gc.C) {
	s.testNewCommandsString(c,
		commandArgs("anything", -1, ""),
		"run commands",
	)
}

func (s *FactorySuite) TestNewCommandsString_WithRelation(c *gc.C) {
	s.testNewCommandsString(c,
		commandArgs("anything", 0, ""),
		"run commands (0)",
	)
}

func (s *FactorySuite) TestNewCommandsString_WithRelationAndUnit(c *gc.C) {
	s.testNewCommandsString(c,
		commandArgs("anything", 3, "unit/123"),
		"run commands (3; unit/123)",
	)
}

func (s *FactorySuite) testNewHookError(c *gc.C, newHook newHook) {
	op, err := newHook(s.factory, hook.Info{Kind: hooks.Kind("gibberish")})
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `unknown hook kind "gibberish"`)
}

func (s *FactorySuite) TestNewHookError_Run(c *gc.C) {
	s.testNewHookError(c, (operation.Factory).NewRunHook)
}

func (s *FactorySuite) TestNewHookError_Retry(c *gc.C) {
	s.testNewHookError(c, (operation.Factory).NewRetryHook)
}

func (s *FactorySuite) TestNewHookError_Skip(c *gc.C) {
	s.testNewHookError(c, (operation.Factory).NewSkipHook)
}

func (s *FactorySuite) TestNewHookString_Run(c *gc.C) {
	op, err := s.factory.NewRunHook(hook.Info{Kind: hooks.Install})
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "run install hook")
}

func (s *FactorySuite) TestNewHookString_Retry(c *gc.C) {
	op, err := s.factory.NewRetryHook(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 123,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "clear resolved flag and run relation-broken (123) hook")
}

func (s *FactorySuite) TestNewHookString_Skip(c *gc.C) {
	op, err := s.factory.NewSkipHook(hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "foo/22",
		RelationId: 123,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "clear resolved flag and skip run relation-joined (123; foo/22) hook")
}

func (s *FactorySuite) TestNewUpdateRelationsString(c *gc.C) {
	op, err := s.factory.NewUpdateRelations([]int{1, 2, 3})
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "update relations [1 2 3]")
}

func (s *FactorySuite) TestNewAcceptLeadershipString(c *gc.C) {
	op, err := s.factory.NewAcceptLeadership()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "accept leadership")
}

func (s *FactorySuite) TestNewResignLeadershipString(c *gc.C) {
	op, err := s.factory.NewResignLeadership()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "resign leadership")
}
