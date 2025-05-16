// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
)

type DeploySuite struct {
	testhelpers.IsolationSuite
}

func TestDeploySuite(t *stdtesting.T) { tc.Run(t, &DeploySuite{}) }

type newDeploy func(operation.Factory, string) (operation.Operation, error)

func (s *DeploySuite) testPrepareAlreadyDone(
	c *tc.C, newDeploy newDeploy, kind operation.Kind,
) {
	callbacks := &DeployCallbacks{}
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
		Logger:    loggertesting.WrapCheckLog(c),
	})
	op, err := newDeploy(factory, "ch:quantal/hive-23")
	c.Assert(err, tc.ErrorIsNil)
	newState, err := op.Prepare(c.Context(), operation.State{
		Kind:     kind,
		Step:     operation.Done,
		CharmURL: "ch:quantal/hive-23",
	})
	c.Check(newState, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, operation.ErrSkipExecute)
}

func (s *DeploySuite) TestPrepareAlreadyDone_Install(c *tc.C) {
	s.testPrepareAlreadyDone(c,
		(operation.Factory).NewInstall,
		operation.Install,
	)
}

func (s *DeploySuite) TestPrepareAlreadyDone_Upgrade(c *tc.C) {
	s.testPrepareAlreadyDone(c,
		(operation.Factory).NewUpgrade,
		operation.Upgrade,
	)
}

func (s *DeploySuite) TestPrepareAlreadyDone_RevertUpgrade(c *tc.C) {
	s.testPrepareAlreadyDone(c,
		(operation.Factory).NewRevertUpgrade,
		operation.Upgrade,
	)
}

func (s *DeploySuite) TestPrepareAlreadyDone_ResolvedUpgrade(c *tc.C) {
	s.testPrepareAlreadyDone(c,
		(operation.Factory).NewResolvedUpgrade,
		operation.Upgrade,
	)
}

func (s *DeploySuite) testPrepareArchiveInfoError(c *tc.C, newDeploy newDeploy) {
	callbacks := &DeployCallbacks{
		MockGetArchiveInfo: &MockGetArchiveInfo{err: errors.New("pew")},
	}
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
		Logger:    loggertesting.WrapCheckLog(c),
	})
	op, err := newDeploy(factory, "ch:quantal/hive-23")
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(c.Context(), operation.State{})
	c.Check(newState, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "pew")
	c.Check(callbacks.MockGetArchiveInfo.gotCharmURL, tc.Equals, "ch:quantal/hive-23")
}

func (s *DeploySuite) TestPrepareArchiveInfoError_Install(c *tc.C) {
	s.testPrepareArchiveInfoError(c, (operation.Factory).NewInstall)
}

func (s *DeploySuite) TestPrepareArchiveInfoError_Upgrade(c *tc.C) {
	s.testPrepareArchiveInfoError(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestPrepareArchiveInfoError_RevertUpgrade(c *tc.C) {
	s.testPrepareArchiveInfoError(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestPrepareArchiveInfoError_ResolvedUpgrade(c *tc.C) {
	s.testPrepareArchiveInfoError(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testPrepareStageError(c *tc.C, newDeploy newDeploy) {
	callbacks := &DeployCallbacks{
		MockGetArchiveInfo: &MockGetArchiveInfo{info: &MockBundleInfo{}},
	}
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
		MockStage:          &MockStage{err: errors.New("squish")},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
		Logger:    loggertesting.WrapCheckLog(c),
	})
	op, err := newDeploy(factory, "ch:quantal/hive-23")
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(c.Context(), operation.State{})
	c.Check(newState, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "squish")
	c.Check(*deployer.MockStage.gotInfo, tc.Equals, callbacks.MockGetArchiveInfo.info)
}

func (s *DeploySuite) TestPrepareStageError_Install(c *tc.C) {
	s.testPrepareStageError(c, (operation.Factory).NewInstall)
}

func (s *DeploySuite) TestPrepareStageError_Upgrade(c *tc.C) {
	s.testPrepareStageError(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestPrepareStageError_RevertUpgrade(c *tc.C) {
	s.testPrepareStageError(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestPrepareStageError_ResolvedUpgrade(c *tc.C) {
	s.testPrepareStageError(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testPrepareSetCharmError(c *tc.C, newDeploy newDeploy) {
	callbacks := &DeployCallbacks{
		MockGetArchiveInfo:  &MockGetArchiveInfo{},
		MockSetCurrentCharm: &MockSetCurrentCharm{err: errors.New("blargh")},
	}
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
		MockStage:          &MockStage{},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
		Logger:    loggertesting.WrapCheckLog(c),
	})

	op, err := newDeploy(factory, "ch:quantal/hive-23")
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(c.Context(), operation.State{})
	c.Check(newState, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "blargh")
	c.Check(callbacks.MockSetCurrentCharm.gotCharmURL, tc.Equals, "ch:quantal/hive-23")
}

func (s *DeploySuite) TestPrepareSetCharmError_Install(c *tc.C) {
	s.testPrepareSetCharmError(c, (operation.Factory).NewInstall)
}

func (s *DeploySuite) TestPrepareSetCharmError_Upgrade(c *tc.C) {
	s.testPrepareSetCharmError(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestPrepareSetCharmError_RevertUpgrade(c *tc.C) {
	s.testPrepareSetCharmError(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestPrepareSetCharmError_ResolvedUpgrade(c *tc.C) {
	s.testPrepareSetCharmError(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testPrepareSuccess(c *tc.C, newDeploy newDeploy, before, after operation.State) {
	callbacks := NewDeployCallbacks()
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
		MockStage:          &MockStage{},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
		Logger:    loggertesting.WrapCheckLog(c),
	})
	op, err := newDeploy(factory, "ch:quantal/nyancat-4")
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(c.Context(), before)
	c.Check(err, tc.ErrorIsNil)
	c.Check(newState, tc.DeepEquals, &after)
	c.Check(callbacks.MockSetCurrentCharm.gotCharmURL, tc.Equals, "ch:quantal/nyancat-4")
}

func (s *DeploySuite) TestPrepareSuccess_Install_BlankSlate(c *tc.C) {
	s.testPrepareSuccess(c,
		(operation.Factory).NewInstall,
		operation.State{},
		operation.State{
			Kind:     operation.Install,
			Step:     operation.Pending,
			CharmURL: "ch:quantal/nyancat-4",
		},
	)
}

func (s *DeploySuite) TestPrepareSuccess_Install_Queued(c *tc.C) {
	s.testPrepareSuccess(c,
		(operation.Factory).NewInstall,
		operation.State{
			Kind:     operation.Install,
			Step:     operation.Queued,
			CharmURL: "ch:quantal/nyancat-4",
		},
		operation.State{
			Kind:     operation.Install,
			Step:     operation.Pending,
			CharmURL: "ch:quantal/nyancat-4",
		},
	)
}

func (s *DeploySuite) TestPrepareSuccess_Upgrade_PreservePendingHook(c *tc.C) {
	for i, newDeploy := range []newDeploy{
		(operation.Factory).NewUpgrade,
		(operation.Factory).NewRevertUpgrade,
		(operation.Factory).NewResolvedUpgrade,
	} {
		c.Logf("variant %d", i)
		s.testPrepareSuccess(c,
			newDeploy,
			operation.State{
				Kind: operation.RunHook,
				Step: operation.Pending,
				Hook: &hook.Info{Kind: hooks.ConfigChanged},
			},
			operation.State{
				Kind:     operation.Upgrade,
				Step:     operation.Pending,
				CharmURL: "ch:quantal/nyancat-4",
				Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			},
		)
	}
}

func (s *DeploySuite) TestPrepareSuccess_Upgrade_PreserveOriginalPendingHook(c *tc.C) {
	for i, newDeploy := range []newDeploy{
		(operation.Factory).NewUpgrade,
		(operation.Factory).NewRevertUpgrade,
		(operation.Factory).NewResolvedUpgrade,
	} {
		c.Logf("variant %d", i)
		s.testPrepareSuccess(c,
			newDeploy,
			operation.State{
				Kind:     operation.Upgrade,
				Step:     operation.Pending,
				CharmURL: "ch:quantal/random-23",
				Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			},
			operation.State{
				Kind:     operation.Upgrade,
				Step:     operation.Pending,
				CharmURL: "ch:quantal/nyancat-4",
				Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			},
		)
	}
}

func (s *DeploySuite) TestPrepareSuccess_Upgrade_PreserveNoHook(c *tc.C) {
	for i, newDeploy := range []newDeploy{
		(operation.Factory).NewUpgrade,
		(operation.Factory).NewRevertUpgrade,
		(operation.Factory).NewResolvedUpgrade,
	} {
		c.Logf("variant %d", i)
		s.testPrepareSuccess(c,
			newDeploy,
			overwriteState,
			operation.State{
				Kind:     operation.Upgrade,
				Step:     operation.Pending,
				CharmURL: "ch:quantal/nyancat-4",
				Started:  true,
			},
		)
	}
}

func (s *DeploySuite) TestExecuteConflictError_Install(c *tc.C) {
	s.testExecuteError(c, (operation.Factory).NewInstall)
}

func (s *DeploySuite) TestExecuteConflictError_Upgrade(c *tc.C) {
	s.testExecuteError(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestExecuteConflictError_RevertUpgrade(c *tc.C) {
	s.testExecuteError(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestExecuteConflictError_ResolvedUpgrade(c *tc.C) {
	s.testExecuteError(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testExecuteError(c *tc.C, newDeploy newDeploy) {
	callbacks := NewDeployCallbacks()
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
		MockStage:          &MockStage{},
		MockDeploy:         &MockNoArgs{err: errors.New("rasp")},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
		Logger:    loggertesting.WrapCheckLog(c),
	})
	op, err := newDeploy(factory, "ch:quantal/nyancat-4")
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Prepare(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Execute(c.Context(), operation.State{})
	c.Check(newState, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "rasp")
	c.Check(deployer.MockDeploy.called, tc.IsTrue)
}

func (s *DeploySuite) TestExecuteError_Install(c *tc.C) {
	s.testExecuteError(c, (operation.Factory).NewInstall)
}

func (s *DeploySuite) TestExecuteError_Upgrade(c *tc.C) {
	s.testExecuteError(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestExecuteError_RevertUpgrade(c *tc.C) {
	s.testExecuteError(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestExecuteError_ResolvedUpgrade(c *tc.C) {
	s.testExecuteError(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testExecuteSuccess(
	c *tc.C, newDeploy newDeploy, before, after operation.State,
) {
	deployer := NewMockDeployer()
	callbacks := NewDeployCallbacks()
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
		Logger:    loggertesting.WrapCheckLog(c),
	})
	op, err := newDeploy(factory, "ch:quantal/lol-1")
	c.Assert(err, tc.ErrorIsNil)

	midState, err := op.Prepare(c.Context(), before)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(midState, tc.NotNil)

	newState, err := op.Execute(c.Context(), *midState)
	c.Check(err, tc.ErrorIsNil)
	c.Check(newState, tc.DeepEquals, &after)
	c.Check(deployer.MockDeploy.called, tc.IsTrue)
}

func (s *DeploySuite) TestExecuteSuccess_Install_BlankSlate(c *tc.C) {
	s.testExecuteSuccess(c,
		(operation.Factory).NewInstall,
		operation.State{},
		operation.State{
			Kind:     operation.Install,
			Step:     operation.Done,
			CharmURL: "ch:quantal/lol-1",
		},
	)
}

func (s *DeploySuite) TestExecuteSuccess_Install_Queued(c *tc.C) {
	s.testExecuteSuccess(c,
		(operation.Factory).NewInstall,
		operation.State{
			Kind:     operation.Install,
			Step:     operation.Queued,
			CharmURL: "ch:quantal/lol-1",
		},
		operation.State{
			Kind:     operation.Install,
			Step:     operation.Done,
			CharmURL: "ch:quantal/lol-1",
		},
	)
}

func (s *DeploySuite) TestExecuteSuccess_Upgrade_PreservePendingHook(c *tc.C) {
	for i, newDeploy := range []newDeploy{
		(operation.Factory).NewUpgrade,
		(operation.Factory).NewRevertUpgrade,
		(operation.Factory).NewResolvedUpgrade,
	} {
		c.Logf("variant %d", i)
		s.testExecuteSuccess(c,
			newDeploy,
			operation.State{
				Kind: operation.RunHook,
				Step: operation.Pending,
				Hook: &hook.Info{Kind: hooks.ConfigChanged},
			},
			operation.State{
				Kind:     operation.Upgrade,
				Step:     operation.Done,
				CharmURL: "ch:quantal/lol-1",
				Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			},
		)
	}
}

func (s *DeploySuite) TestExecuteSuccess_Upgrade_PreserveOriginalPendingHook(c *tc.C) {
	for i, newDeploy := range []newDeploy{
		(operation.Factory).NewUpgrade,
		(operation.Factory).NewRevertUpgrade,
		(operation.Factory).NewResolvedUpgrade,
	} {
		c.Logf("variant %d", i)
		s.testExecuteSuccess(c,
			newDeploy,
			operation.State{
				Kind:     operation.Upgrade,
				Step:     operation.Pending,
				CharmURL: "ch:quantal/wild-9",
				Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			},
			operation.State{
				Kind:     operation.Upgrade,
				Step:     operation.Done,
				CharmURL: "ch:quantal/lol-1",
				Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			},
		)
	}
}

func (s *DeploySuite) TestExecuteSuccess_Upgrade_PreserveNoHook(c *tc.C) {
	for i, newDeploy := range []newDeploy{
		(operation.Factory).NewUpgrade,
		(operation.Factory).NewRevertUpgrade,
		(operation.Factory).NewResolvedUpgrade,
	} {
		c.Logf("variant %d", i)
		s.testExecuteSuccess(c,
			newDeploy,
			overwriteState,
			operation.State{
				Kind:     operation.Upgrade,
				Step:     operation.Done,
				CharmURL: "ch:quantal/lol-1",
				Started:  true,
			},
		)
	}
}

func (s *DeploySuite) TestCommitQueueInstallHook(c *tc.C) {
	callbacks := NewDeployCommitCallbacks(nil)
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
		Logger:    loggertesting.WrapCheckLog(c),
	})
	op, err := factory.NewInstall("ch:quantal/x-0")
	c.Assert(err, tc.ErrorIsNil)
	newState, err := op.Commit(c.Context(), operation.State{
		Kind:     operation.Install,
		Step:     operation.Done,
		CharmURL: "", // doesn't actually matter here
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(newState, tc.DeepEquals, &operation.State{
		Kind: operation.RunHook,
		Step: operation.Queued,
		Hook: &hook.Info{Kind: hooks.Install},
	})
}

func (s *DeploySuite) testCommitQueueUpgradeHook(c *tc.C, newDeploy newDeploy) {
	callbacks := NewDeployCommitCallbacks(nil)
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
		Logger:    loggertesting.WrapCheckLog(c),
	})

	op, err := newDeploy(factory, "ch:quantal/x-0")
	c.Assert(err, tc.ErrorIsNil)
	newState, err := op.Commit(c.Context(), operation.State{
		Kind:     operation.Upgrade,
		Step:     operation.Done,
		CharmURL: "", // doesn't actually matter here
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(newState, tc.DeepEquals, &operation.State{
		Kind: operation.RunHook,
		Step: operation.Queued,
		Hook: &hook.Info{Kind: hooks.UpgradeCharm},
	})
}

func (s *DeploySuite) TestCommitQueueUpgradeHook_Upgrade(c *tc.C) {
	s.testCommitQueueUpgradeHook(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestCommitQueueUpgradeHook_RevertUpgrade(c *tc.C) {
	s.testCommitQueueUpgradeHook(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestCommitQueueUpgradeHook_ResolvedUpgrade(c *tc.C) {
	s.testCommitQueueUpgradeHook(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testCommitInterruptedHook(c *tc.C, newDeploy newDeploy) {
	callbacks := NewDeployCommitCallbacks(nil)
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
		Logger:    loggertesting.WrapCheckLog(c),
	})

	op, err := newDeploy(factory, "ch:quantal/x-0")
	c.Assert(err, tc.ErrorIsNil)
	hookStep := operation.Done
	newState, err := op.Commit(c.Context(), operation.State{
		Kind:     operation.Upgrade,
		Step:     operation.Done,
		CharmURL: "", // doesn't actually matter here
		Hook:     &hook.Info{Kind: hooks.ConfigChanged},
		HookStep: &hookStep,
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(newState, tc.DeepEquals, &operation.State{
		Kind:     operation.RunHook,
		Step:     operation.Pending,
		Hook:     &hook.Info{Kind: hooks.ConfigChanged},
		HookStep: &hookStep,
	})
}

func (s *DeploySuite) TestCommitInterruptedHook_Upgrade(c *tc.C) {
	s.testCommitInterruptedHook(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestCommitInterruptedHook_RevertUpgrade(c *tc.C) {
	s.testCommitInterruptedHook(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestCommitInterruptedHook_ResolvedUpgrade(c *tc.C) {
	s.testCommitInterruptedHook(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testDoesNotNeedGlobalMachineLock(c *tc.C, newDeploy newDeploy) {
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer: deployer,
		Logger:   loggertesting.WrapCheckLog(c),
	})
	op, err := newDeploy(factory, "ch:quantal/x-0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), tc.IsFalse)
}

func (s *DeploySuite) TestDoesNotNeedGlobalMachineLock_Install(c *tc.C) {
	s.testDoesNotNeedGlobalMachineLock(c, (operation.Factory).NewInstall)
}

func (s *DeploySuite) TestDoesNotNeedGlobalMachineLock_Upgrade(c *tc.C) {
	s.testDoesNotNeedGlobalMachineLock(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestDoesNotNeedGlobalMachineLock_RevertUpgrade(c *tc.C) {
	s.testDoesNotNeedGlobalMachineLock(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestDoesNotNeedGlobalMachineLock_ResolvedUpgrade(c *tc.C) {
	s.testDoesNotNeedGlobalMachineLock(c, (operation.Factory).NewResolvedUpgrade)
}
