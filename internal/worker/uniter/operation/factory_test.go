// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	utilexec "github.com/juju/utils/v4/exec"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/rpc/params"
)

type FactorySuite struct {
	testhelpers.IsolationSuite
	factory   operation.Factory
	actionErr *params.Error
}

func TestFactorySuite(t *stdtesting.T) {
	tc.Run(t, &FactorySuite{})
}

func (s *FactorySuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	// Yes, this factory will produce useless ops; this suite is just for
	// verifying that inadequate args to the factory methods will produce
	// the expected errors; and that the results of same get a string
	// representation that does not depend on the factory attributes.
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
	}
	s.actionErr = nil
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		actionResult := params.ActionResult{
			Action: &params.Action{Name: "backup"},
		}
		if s.actionErr != nil {
			actionResult = params.ActionResult{Error: s.actionErr}
		}
		*(result.(*params.ActionResults)) = params.ActionResults{
			Results: []params.ActionResult{actionResult},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	s.factory = operation.NewFactory(operation.FactoryParams{
		ActionGetter: client,
		Deployer:     deployer,
		Logger:       loggertesting.WrapCheckLog(c),
	})
}

func (s *FactorySuite) testNewDeployError(c *tc.C, newDeploy newDeploy) {
	op, err := newDeploy(s.factory, "")
	c.Check(op, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "charm url required")
}

func (s *FactorySuite) TestNewInstallError(c *tc.C) {
	s.testNewDeployError(c, (operation.Factory).NewInstall)
}

func (s *FactorySuite) TestNewUpgradeError(c *tc.C) {
	s.testNewDeployError(c, (operation.Factory).NewUpgrade)
}

func (s *FactorySuite) TestNewRevertUpgradeError(c *tc.C) {
	s.testNewDeployError(c, (operation.Factory).NewRevertUpgrade)
}

func (s *FactorySuite) TestNewResolvedUpgradeError(c *tc.C) {
	s.testNewDeployError(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *FactorySuite) testNewDeployString(c *tc.C, newDeploy newDeploy, expectPrefix string) {
	op, err := newDeploy(s.factory, "ch:quantal/wordpress-1")
	c.Check(err, tc.ErrorIsNil)
	c.Check(op.String(), tc.Equals, expectPrefix+" ch:quantal/wordpress-1")
}

func (s *FactorySuite) TestNewInstallString(c *tc.C) {
	s.testNewDeployString(c, (operation.Factory).NewInstall, "install")
}

func (s *FactorySuite) TestNewUpgradeString(c *tc.C) {
	s.testNewDeployString(c, (operation.Factory).NewUpgrade, "upgrade to")
}

func (s *FactorySuite) TestNewRevertUpgradeString(c *tc.C) {
	s.testNewDeployString(c,
		(operation.Factory).NewRevertUpgrade,
		"switch upgrade to",
	)
}

func (s *FactorySuite) TestNewResolvedUpgradeString(c *tc.C) {
	s.testNewDeployString(c,
		(operation.Factory).NewResolvedUpgrade,
		"continue upgrade to",
	)
}

func (s *FactorySuite) TestNewActionError(c *tc.C) {
	op, err := s.factory.NewAction(c.Context(), "lol-something")
	c.Check(op, tc.IsNil)
	c.Check(err, tc.ErrorMatches, `invalid action id "lol-something"`)
}

func (s *FactorySuite) TestNewActionString(c *tc.C) {
	op, err := s.factory.NewAction(c.Context(), someActionId)
	c.Check(err, tc.ErrorIsNil)
	c.Check(op.String(), tc.Equals, "run action "+someActionId)
}

func panicSendResponse(*utilexec.ExecResponse, error) bool {
	panic("don't call this")
}

func commandArgs(commands string, relationId int, remoteUnit string) operation.CommandArgs {
	return operation.CommandArgs{
		Commands:       commands,
		RelationId:     relationId,
		RemoteUnitName: remoteUnit,
		// TODO(jam): 2019-10-24 Include RemoteAppName
	}
}

func (s *FactorySuite) TestNewCommandsSendResponseError(c *tc.C) {
	op, err := s.factory.NewCommands(commandArgs("anything", -1, ""), nil)
	c.Check(op, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "response sender required")
}

func (s *FactorySuite) testNewCommandsArgsError(
	c *tc.C, args operation.CommandArgs, expect string,
) {
	op, err := s.factory.NewCommands(args, panicSendResponse)
	c.Check(op, tc.IsNil)
	c.Check(err, tc.ErrorMatches, expect)
}

func (s *FactorySuite) TestNewCommandsArgsError_NoCommand(c *tc.C) {
	s.testNewCommandsArgsError(c,
		commandArgs("", -1, ""),
		"commands required",
	)
}

func (s *FactorySuite) TestNewCommandsArgsError_BadRemoteUnit(c *tc.C) {
	s.testNewCommandsArgsError(c,
		commandArgs("any old thing", -1, "unit/1"),
		"remote unit not valid without relation",
	)
}

func (s *FactorySuite) TestNewCommandsArgsError_BadRemoteUnitName(c *tc.C) {
	s.testNewCommandsArgsError(c,
		commandArgs("any old thing", 0, "lol"),
		`invalid remote unit name "lol"`,
	)
}

func (s *FactorySuite) testNewCommandsString(
	c *tc.C, args operation.CommandArgs, expect string,
) {
	op, err := s.factory.NewCommands(args, panicSendResponse)
	c.Check(err, tc.ErrorIsNil)
	c.Check(op.String(), tc.Equals, expect)
}

func (s *FactorySuite) TestNewCommandsString_CommandsOnly(c *tc.C) {
	s.testNewCommandsString(c,
		commandArgs("anything", -1, ""),
		"run commands",
	)
}

func (s *FactorySuite) TestNewCommandsString_WithRelation(c *tc.C) {
	s.testNewCommandsString(c,
		commandArgs("anything", 0, ""),
		"run commands (0)",
	)
}

func (s *FactorySuite) TestNewCommandsString_WithRelationAndUnit(c *tc.C) {
	s.testNewCommandsString(c,
		commandArgs("anything", 3, "unit/123"),
		"run commands (3; unit/123)",
	)
}

func (s *FactorySuite) testNewHookError(c *tc.C, newHook newHook) {
	op, err := newHook(s.factory, hook.Info{Kind: hooks.Kind("gibberish")})
	c.Check(op, tc.IsNil)
	c.Check(err, tc.ErrorMatches, `unknown hook kind "gibberish"`)
}

func (s *FactorySuite) TestNewHookError_Run(c *tc.C) {
	s.testNewHookError(c, (operation.Factory).NewRunHook)
}

func (s *FactorySuite) TestNewHookError_Skip(c *tc.C) {
	s.testNewHookError(c, (operation.Factory).NewSkipHook)
}

func (s *FactorySuite) TestNewHookString_Run(c *tc.C) {
	op, err := s.factory.NewRunHook(hook.Info{Kind: hooks.Install})
	c.Check(err, tc.ErrorIsNil)
	c.Check(op.String(), tc.Equals, "run install hook")
}

func (s *FactorySuite) TestNewHookString_Skip(c *tc.C) {
	op, err := s.factory.NewSkipHook(hook.Info{
		Kind:              hooks.RelationJoined,
		RemoteUnit:        "foo/22",
		RemoteApplication: "foo/22",
		RelationId:        123,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(op.String(), tc.Equals, "skip run relation-joined (123; unit: foo/22) hook")
}

func (s *FactorySuite) TestNewAcceptLeadershipString(c *tc.C) {
	op, err := s.factory.NewAcceptLeadership()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "accept leadership")
}

func (s *FactorySuite) TestNewResignLeadershipString(c *tc.C) {
	op, err := s.factory.NewResignLeadership()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "resign leadership")
}

func (s *FactorySuite) TestNewActionNotAvailable(c *tc.C) {
	s.actionErr = &params.Error{Code: "action no longer available"}
	rnr, err := s.factory.NewAction(c.Context(), "666")
	c.Assert(rnr, tc.IsNil)
	c.Assert(err, tc.Equals, charmrunner.ErrActionNotAvailable)
}

func (s *FactorySuite) TestNewActionUnauthorised(c *tc.C) {
	s.actionErr = &params.Error{Code: "unauthorized access"}
	rnr, err := s.factory.NewAction(c.Context(), "666")
	c.Assert(rnr, tc.IsNil)
	c.Assert(err, tc.Equals, charmrunner.ErrActionNotAvailable)
}

func (s *FactorySuite) TestNewNoOpSecretsRemoved(c *tc.C) {
	op, err := s.factory.NewNoOpSecretsRemoved([]string{"secreturi"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "process removed secrets: [secreturi]")
}
