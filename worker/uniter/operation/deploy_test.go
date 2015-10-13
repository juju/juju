// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type DeploySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DeploySuite{})

type newDeploy func(operation.Factory, *corecharm.URL) (operation.Operation, error)

func (s *DeploySuite) testPrepareAlreadyDone(
	c *gc.C, newDeploy newDeploy, kind operation.Kind,
) {
	callbacks := &DeployCallbacks{}
	factory := operation.NewFactory(operation.FactoryParams{Callbacks: callbacks})
	op, err := newDeploy(factory, curl("cs:quantal/hive-23"))
	c.Assert(err, jc.ErrorIsNil)
	newState, err := op.Prepare(operation.State{
		Kind:     kind,
		Step:     operation.Done,
		CharmURL: curl("cs:quantal/hive-23"),
	})
	c.Check(newState, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, operation.ErrSkipExecute)
}

func (s *DeploySuite) TestPrepareAlreadyDone_Install(c *gc.C) {
	s.testPrepareAlreadyDone(c,
		(operation.Factory).NewInstall,
		operation.Install,
	)
}

func (s *DeploySuite) TestPrepareAlreadyDone_Upgrade(c *gc.C) {
	s.testPrepareAlreadyDone(c,
		(operation.Factory).NewUpgrade,
		operation.Upgrade,
	)
}

func (s *DeploySuite) TestPrepareAlreadyDone_RevertUpgrade(c *gc.C) {
	s.testPrepareAlreadyDone(c,
		(operation.Factory).NewRevertUpgrade,
		operation.Upgrade,
	)
}

func (s *DeploySuite) TestPrepareAlreadyDone_ResolvedUpgrade(c *gc.C) {
	s.testPrepareAlreadyDone(c,
		(operation.Factory).NewResolvedUpgrade,
		operation.Upgrade,
	)
}

func (s *DeploySuite) testNotifyDeployerError(
	c *gc.C, newDeploy newDeploy, expectNotifyRevert bool,
) {
	callbacks := &DeployCallbacks{}
	deployer := &MockDeployer{}
	expectCall := &MockNoArgs{err: errors.New("snh")}
	if expectNotifyRevert {
		deployer.MockNotifyRevert = expectCall
	} else {
		deployer.MockNotifyResolved = expectCall
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Deployer:  deployer,
	})
	op, err := newDeploy(factory, curl("cs:quantal/hive-23"))
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Check(newState, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "snh")
	c.Check(expectCall.called, jc.IsTrue)
}

func (s *DeploySuite) TestNotifyDeployerError_RevertUpgrade(c *gc.C) {
	s.testNotifyDeployerError(c, (operation.Factory).NewRevertUpgrade, true)
}

func (s *DeploySuite) TestNotifyDeployerError_ResolvedUpgrade(c *gc.C) {
	s.testNotifyDeployerError(c, (operation.Factory).NewResolvedUpgrade, false)
}

func (s *DeploySuite) testPrepareArchiveInfoError(c *gc.C, newDeploy newDeploy) {
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
	})
	op, err := newDeploy(factory, curl("cs:quantal/hive-23"))
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Check(newState, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "pew")
	c.Check(callbacks.MockGetArchiveInfo.gotCharmURL, gc.DeepEquals, curl("cs:quantal/hive-23"))
}

func (s *DeploySuite) TestPrepareArchiveInfoError_Install(c *gc.C) {
	s.testPrepareArchiveInfoError(c, (operation.Factory).NewInstall)
}

func (s *DeploySuite) TestPrepareArchiveInfoError_Upgrade(c *gc.C) {
	s.testPrepareArchiveInfoError(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestPrepareArchiveInfoError_RevertUpgrade(c *gc.C) {
	s.testPrepareArchiveInfoError(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestPrepareArchiveInfoError_ResolvedUpgrade(c *gc.C) {
	s.testPrepareArchiveInfoError(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testPrepareStageError(c *gc.C, newDeploy newDeploy) {
	callbacks := &DeployCallbacks{
		MockGetArchiveInfo: &MockGetArchiveInfo{info: &MockBundleInfo{}},
	}
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
		MockStage:          &MockStage{err: errors.New("squish")},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
		Abort:     abort,
	})
	op, err := newDeploy(factory, curl("cs:quantal/hive-23"))
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Check(newState, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "squish")
	c.Check(*deployer.MockStage.gotInfo, gc.Equals, callbacks.MockGetArchiveInfo.info)
	c.Check(*deployer.MockStage.gotAbort, gc.Equals, abort)
}

func (s *DeploySuite) TestPrepareStageError_Install(c *gc.C) {
	s.testPrepareStageError(c, (operation.Factory).NewInstall)
}

func (s *DeploySuite) TestPrepareStageError_Upgrade(c *gc.C) {
	s.testPrepareStageError(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestPrepareStageError_RevertUpgrade(c *gc.C) {
	s.testPrepareStageError(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestPrepareStageError_ResolvedUpgrade(c *gc.C) {
	s.testPrepareStageError(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testPrepareSetCharmError(c *gc.C, newDeploy newDeploy) {
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
	})

	op, err := newDeploy(factory, curl("cs:quantal/hive-23"))
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Check(newState, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "blargh")
	c.Check(callbacks.MockSetCurrentCharm.gotCharmURL, gc.DeepEquals, curl("cs:quantal/hive-23"))
}

func (s *DeploySuite) TestPrepareSetCharmError_Install(c *gc.C) {
	s.testPrepareSetCharmError(c, (operation.Factory).NewInstall)
}

func (s *DeploySuite) TestPrepareSetCharmError_Upgrade(c *gc.C) {
	s.testPrepareSetCharmError(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestPrepareSetCharmError_RevertUpgrade(c *gc.C) {
	s.testPrepareSetCharmError(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestPrepareSetCharmError_ResolvedUpgrade(c *gc.C) {
	s.testPrepareSetCharmError(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testPrepareSuccess(c *gc.C, newDeploy newDeploy, before, after operation.State) {
	callbacks := NewDeployCallbacks()
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
		MockStage:          &MockStage{},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
	})
	op, err := newDeploy(factory, curl("cs:quantal/nyancat-4"))
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(before)
	c.Check(err, jc.ErrorIsNil)
	c.Check(newState, gc.DeepEquals, &after)
	c.Check(callbacks.MockSetCurrentCharm.gotCharmURL, gc.DeepEquals, curl("cs:quantal/nyancat-4"))
}

func (s *DeploySuite) TestPrepareSuccess_Install_BlankSlate(c *gc.C) {
	s.testPrepareSuccess(c,
		(operation.Factory).NewInstall,
		operation.State{},
		operation.State{
			Kind:     operation.Install,
			Step:     operation.Pending,
			CharmURL: curl("cs:quantal/nyancat-4"),
		},
	)
}

func (s *DeploySuite) TestPrepareSuccess_Install_Queued(c *gc.C) {
	s.testPrepareSuccess(c,
		(operation.Factory).NewInstall,
		operation.State{
			Kind:     operation.Install,
			Step:     operation.Queued,
			CharmURL: curl("cs:quantal/nyancat-4"),
		},
		operation.State{
			Kind:     operation.Install,
			Step:     operation.Pending,
			CharmURL: curl("cs:quantal/nyancat-4"),
		},
	)
}

func (s *DeploySuite) TestPrepareSuccess_Upgrade_PreservePendingHook(c *gc.C) {
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
				CharmURL: curl("cs:quantal/nyancat-4"),
				Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			},
		)
	}
}

func (s *DeploySuite) TestPrepareSuccess_Upgrade_PreserveOriginalPendingHook(c *gc.C) {
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
				CharmURL: curl("cs:quantal/random-23"),
				Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			},
			operation.State{
				Kind:     operation.Upgrade,
				Step:     operation.Pending,
				CharmURL: curl("cs:quantal/nyancat-4"),
				Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			},
		)
	}
}

func (s *DeploySuite) TestPrepareSuccess_Upgrade_PreserveNoHook(c *gc.C) {
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
				CharmURL: curl("cs:quantal/nyancat-4"),
				Started:  true,
			},
		)
	}
}

func (s *DeploySuite) testExecuteConflictError(c *gc.C, newDeploy newDeploy) {
	callbacks := NewDeployCallbacks()
	deployer := &MockDeployer{
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
		MockStage:          &MockStage{},
		MockDeploy:         &MockNoArgs{err: charm.ErrConflict},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
	})
	charmURL := curl("cs:quantal/nyancat-4")
	op, err := newDeploy(factory, charmURL)
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Check(newState, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "cannot deploy charm cs:quantal/nyancat-4")
	ok := operation.IsDeployConflictError(err)
	c.Check(ok, jc.IsTrue)
	c.Check(deployer.MockDeploy.called, jc.IsTrue)
}

func (s *DeploySuite) TestExecuteConflictError_Install(c *gc.C) {
	s.testExecuteError(c, (operation.Factory).NewInstall)
}

func (s *DeploySuite) TestExecuteConflictError_Upgrade(c *gc.C) {
	s.testExecuteError(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestExecuteConflictError_RevertUpgrade(c *gc.C) {
	s.testExecuteError(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestExecuteConflictError_ResolvedUpgrade(c *gc.C) {
	s.testExecuteError(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testExecuteError(c *gc.C, newDeploy newDeploy) {
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
	})
	op, err := newDeploy(factory, curl("cs:quantal/nyancat-4"))
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Check(newState, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "rasp")
	c.Check(deployer.MockDeploy.called, jc.IsTrue)
}

func (s *DeploySuite) TestExecuteError_Install(c *gc.C) {
	s.testExecuteError(c, (operation.Factory).NewInstall)
}

func (s *DeploySuite) TestExecuteError_Upgrade(c *gc.C) {
	s.testExecuteError(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestExecuteError_RevertUpgrade(c *gc.C) {
	s.testExecuteError(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestExecuteError_ResolvedUpgrade(c *gc.C) {
	s.testExecuteError(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testExecuteSuccess(
	c *gc.C, newDeploy newDeploy, before, after operation.State,
) {
	deployer := NewMockDeployer()
	callbacks := NewDeployCallbacks()
	factory := operation.NewFactory(operation.FactoryParams{
		Deployer:  deployer,
		Callbacks: callbacks,
	})
	op, err := newDeploy(factory, curl("cs:quantal/lol-1"))
	c.Assert(err, jc.ErrorIsNil)

	midState, err := op.Prepare(before)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(midState, gc.NotNil)

	newState, err := op.Execute(*midState)
	c.Check(err, jc.ErrorIsNil)
	c.Check(newState, gc.DeepEquals, &after)
	c.Check(deployer.MockDeploy.called, jc.IsTrue)
}

func (s *DeploySuite) TestExecuteSuccess_Install_BlankSlate(c *gc.C) {
	s.testExecuteSuccess(c,
		(operation.Factory).NewInstall,
		operation.State{},
		operation.State{
			Kind:     operation.Install,
			Step:     operation.Done,
			CharmURL: curl("cs:quantal/lol-1"),
		},
	)
}

func (s *DeploySuite) TestExecuteSuccess_Install_Queued(c *gc.C) {
	s.testExecuteSuccess(c,
		(operation.Factory).NewInstall,
		operation.State{
			Kind:     operation.Install,
			Step:     operation.Queued,
			CharmURL: curl("cs:quantal/lol-1"),
		},
		operation.State{
			Kind:     operation.Install,
			Step:     operation.Done,
			CharmURL: curl("cs:quantal/lol-1"),
		},
	)
}

func (s *DeploySuite) TestExecuteSuccess_Upgrade_PreservePendingHook(c *gc.C) {
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
				CharmURL: curl("cs:quantal/lol-1"),
				Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			},
		)
	}
}

func (s *DeploySuite) TestExecuteSuccess_Upgrade_PreserveOriginalPendingHook(c *gc.C) {
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
				CharmURL: curl("cs:quantal/wild-9"),
				Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			},
			operation.State{
				Kind:     operation.Upgrade,
				Step:     operation.Done,
				CharmURL: curl("cs:quantal/lol-1"),
				Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			},
		)
	}
}

func (s *DeploySuite) TestExecuteSuccess_Upgrade_PreserveNoHook(c *gc.C) {
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
				CharmURL: curl("cs:quantal/lol-1"),
				Started:  true,
			},
		)
	}
}

func (s *DeploySuite) TestCommitQueueInstallHook(c *gc.C) {
	callbacks := NewDeployCommitCallbacks(nil)
	factory := operation.NewFactory(operation.FactoryParams{Callbacks: callbacks})
	op, err := factory.NewInstall(curl("cs:quantal/x-0"))
	c.Assert(err, jc.ErrorIsNil)
	newState, err := op.Commit(operation.State{
		Kind:     operation.Install,
		Step:     operation.Done,
		CharmURL: nil, // doesn't actually matter here
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RunHook,
		Step: operation.Queued,
		Hook: &hook.Info{Kind: hooks.Install},
	})
}

func (s *DeploySuite) testCommitQueueUpgradeHook(c *gc.C, newDeploy newDeploy) {
	callbacks := NewDeployCommitCallbacks(nil)
	factory := operation.NewFactory(operation.FactoryParams{Callbacks: callbacks})
	op, err := newDeploy(factory, curl("cs:quantal/x-0"))
	c.Assert(err, jc.ErrorIsNil)
	newState, err := op.Commit(operation.State{
		Kind:     operation.Upgrade,
		Step:     operation.Done,
		CharmURL: nil, // doesn't actually matter here
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RunHook,
		Step: operation.Queued,
		Hook: &hook.Info{Kind: hooks.UpgradeCharm},
	})
}

func (s *DeploySuite) TestCommitQueueUpgradeHook_Upgrade(c *gc.C) {
	s.testCommitQueueUpgradeHook(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestCommitQueueUpgradeHook_RevertUpgrade(c *gc.C) {
	s.testCommitQueueUpgradeHook(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestCommitQueueUpgradeHook_ResolvedUpgrade(c *gc.C) {
	s.testCommitQueueUpgradeHook(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testCommitInterruptedHook(c *gc.C, newDeploy newDeploy) {
	callbacks := NewDeployCommitCallbacks(nil)
	factory := operation.NewFactory(operation.FactoryParams{Callbacks: callbacks})
	op, err := newDeploy(factory, curl("cs:quantal/x-0"))
	c.Assert(err, jc.ErrorIsNil)
	newState, err := op.Commit(operation.State{
		Kind:     operation.Upgrade,
		Step:     operation.Done,
		CharmURL: nil, // doesn't actually matter here
		Hook:     &hook.Info{Kind: hooks.ConfigChanged},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{Kind: hooks.ConfigChanged},
	})
}

func (s *DeploySuite) TestCommitInterruptedHook_Upgrade(c *gc.C) {
	s.testCommitInterruptedHook(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestCommitInterruptedHook_RevertUpgrade(c *gc.C) {
	s.testCommitInterruptedHook(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestCommitInterruptedHook_ResolvedUpgrade(c *gc.C) {
	s.testCommitInterruptedHook(c, (operation.Factory).NewResolvedUpgrade)
}

func (s *DeploySuite) testDoesNotNeedGlobalMachineLock(c *gc.C, newDeploy newDeploy) {
	factory := operation.NewFactory(operation.FactoryParams{})
	op, err := newDeploy(factory, curl("cs:quantal/x-0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), jc.IsFalse)
}

func (s *DeploySuite) TestDoesNotNeedGlobalMachineLock_Install(c *gc.C) {
	s.testDoesNotNeedGlobalMachineLock(c, (operation.Factory).NewInstall)
}

func (s *DeploySuite) TestDoesNotNeedGlobalMachineLock_Upgrade(c *gc.C) {
	s.testDoesNotNeedGlobalMachineLock(c, (operation.Factory).NewUpgrade)
}

func (s *DeploySuite) TestDoesNotNeedGlobalMachineLock_RevertUpgrade(c *gc.C) {
	s.testDoesNotNeedGlobalMachineLock(c, (operation.Factory).NewRevertUpgrade)
}

func (s *DeploySuite) TestDoesNotNeedGlobalMachineLock_ResolvedUpgrade(c *gc.C) {
	s.testDoesNotNeedGlobalMachineLock(c, (operation.Factory).NewResolvedUpgrade)
}
