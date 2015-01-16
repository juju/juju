// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v4"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type DeploySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DeploySuite{})

type newDeploy func(operation.Factory, *corecharm.URL) (operation.Operation, error)

func newInstall(factory operation.Factory, charmURL *corecharm.URL) (operation.Operation, error) {
	return factory.NewInstall(charmURL)
}

func newUpgrade(factory operation.Factory, charmURL *corecharm.URL) (operation.Operation, error) {
	return factory.NewUpgrade(charmURL)
}

func newRevertUpgrade(factory operation.Factory, charmURL *corecharm.URL) (operation.Operation, error) {
	return factory.NewRevertUpgrade(charmURL)
}

func newResolvedUpgrade(factory operation.Factory, charmURL *corecharm.URL) (operation.Operation, error) {
	return factory.NewResolvedUpgrade(charmURL)
}

type deployInfo struct {
	newDeploy               newDeploy
	expectKind              operation.Kind
	expectNotifyRevert      bool
	expectNotifyResolved    bool
	expectClearResolvedFlag bool
}

var allDeployInfo = []deployInfo{{
	newDeploy:  newInstall,
	expectKind: operation.Install,
}, {
	newDeploy:  newUpgrade,
	expectKind: operation.Upgrade,
}, {
	newDeploy:               newRevertUpgrade,
	expectKind:              operation.Upgrade,
	expectNotifyRevert:      true,
	expectClearResolvedFlag: true,
}, {
	newDeploy:               newResolvedUpgrade,
	expectKind:              operation.Upgrade,
	expectNotifyResolved:    true,
	expectClearResolvedFlag: true,
}}

func (s *DeploySuite) TestPrepareAlreadyDone(c *gc.C) {
	for i, info := range allDeployInfo {
		c.Logf("test %d", i)
		callbacks := &DeployCallbacks{
			MockClearResolvedFlag: &MockNoArgs{},
		}
		factory := operation.NewFactory(nil, nil, callbacks, nil)
		op, err := info.newDeploy(factory, curl("cs:quantal/hive-23"))
		c.Assert(err, jc.ErrorIsNil)
		newState, err := op.Prepare(operation.State{
			Kind:     info.expectKind,
			Step:     operation.Done,
			CharmURL: curl("cs:quantal/hive-23"),
		})
		c.Check(newState, gc.IsNil)
		c.Check(err, gc.Equals, operation.ErrSkipExecute)
		c.Check(callbacks.MockClearResolvedFlag.called, gc.Equals, info.expectClearResolvedFlag)
	}
}

func (s *DeploySuite) TestClearResolvedFlagError(c *gc.C) {
	for i, info := range allDeployInfo {
		c.Logf("test %d", i)
		if !info.expectClearResolvedFlag {
			continue
		}
		callbacks := &DeployCallbacks{
			MockClearResolvedFlag: &MockNoArgs{err: errors.New("blort")},
		}
		factory := operation.NewFactory(nil, nil, callbacks, nil)
		op, err := info.newDeploy(factory, curl("cs:quantal/hive-23"))
		c.Assert(err, jc.ErrorIsNil)
		newState, err := op.Prepare(operation.State{})
		c.Check(newState, gc.IsNil)
		c.Check(err, gc.ErrorMatches, "blort")
		c.Check(callbacks.MockClearResolvedFlag.called, gc.Equals, info.expectClearResolvedFlag)
	}
}

func (s *DeploySuite) TestNotifyDeployerError(c *gc.C) {
	for i, info := range allDeployInfo {
		c.Logf("test %d", i)
		if !(info.expectNotifyRevert || info.expectNotifyResolved) {
			continue
		}
		callbacks := &DeployCallbacks{
			MockClearResolvedFlag: &MockNoArgs{},
		}
		deployer := &MockDeployer{}
		expectCall := &MockNoArgs{err: errors.New("snh")}
		switch {
		case info.expectNotifyRevert:
			deployer.MockNotifyRevert = expectCall
		case info.expectNotifyResolved:
			deployer.MockNotifyResolved = expectCall
		default:
			continue
		}
		factory := operation.NewFactory(deployer, nil, callbacks, nil)
		op, err := info.newDeploy(factory, curl("cs:quantal/hive-23"))
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Prepare(operation.State{})
		c.Check(newState, gc.IsNil)
		c.Check(err, gc.ErrorMatches, "snh")
		c.Check(expectCall.called, jc.IsTrue)
	}
}

func (s *DeploySuite) TestPrepareArchiveInfoError(c *gc.C) {
	for i, info := range allDeployInfo {
		c.Logf("test %d", i)
		callbacks := &DeployCallbacks{
			MockClearResolvedFlag: &MockNoArgs{},
			MockGetArchiveInfo:    &MockGetArchiveInfo{err: errors.New("pew")},
		}
		deployer := &MockDeployer{
			MockNotifyRevert:   &MockNoArgs{},
			MockNotifyResolved: &MockNoArgs{},
		}
		factory := operation.NewFactory(deployer, nil, callbacks, nil)
		op, err := info.newDeploy(factory, curl("cs:quantal/hive-23"))
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Prepare(operation.State{})
		c.Check(newState, gc.IsNil)
		c.Check(err, gc.ErrorMatches, "pew")
		c.Check(callbacks.MockGetArchiveInfo.gotCharmURL, gc.DeepEquals, curl("cs:quantal/hive-23"))
		c.Check(deployer.MockNotifyRevert.called, gc.Equals, info.expectNotifyRevert)
		c.Check(deployer.MockNotifyResolved.called, gc.Equals, info.expectNotifyResolved)
		c.Check(callbacks.MockClearResolvedFlag.called, gc.Equals, info.expectClearResolvedFlag)
	}
}

func (s *DeploySuite) TestPrepareStageError(c *gc.C) {
	for i, info := range allDeployInfo {
		c.Logf("test %d", i)
		callbacks := &DeployCallbacks{
			MockClearResolvedFlag: &MockNoArgs{},
			MockGetArchiveInfo:    &MockGetArchiveInfo{info: &MockBundleInfo{}},
		}
		deployer := &MockDeployer{
			MockNotifyRevert:   &MockNoArgs{},
			MockNotifyResolved: &MockNoArgs{},
			MockStage:          &MockStage{err: errors.New("squish")},
		}
		var abort <-chan struct{} = make(chan struct{})
		factory := operation.NewFactory(deployer, nil, callbacks, abort)
		op, err := info.newDeploy(factory, curl("cs:quantal/hive-23"))
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Prepare(operation.State{})
		c.Check(newState, gc.IsNil)
		c.Check(err, gc.ErrorMatches, "squish")
		c.Check(*deployer.MockStage.gotInfo, gc.Equals, callbacks.MockGetArchiveInfo.info)
		c.Check(*deployer.MockStage.gotAbort, gc.Equals, abort)
	}
}

func (s *DeploySuite) TestPrepareSetCharmError(c *gc.C) {
	for i, info := range allDeployInfo {
		c.Logf("test %d", i)
		callbacks := &DeployCallbacks{
			MockClearResolvedFlag: &MockNoArgs{},
			MockGetArchiveInfo:    &MockGetArchiveInfo{},
			MockSetCurrentCharm:   &MockSetCurrentCharm{err: errors.New("blargh")},
		}
		deployer := &MockDeployer{
			MockNotifyRevert:   &MockNoArgs{},
			MockNotifyResolved: &MockNoArgs{},
			MockStage:          &MockStage{},
		}
		factory := operation.NewFactory(deployer, nil, callbacks, nil)
		op, err := info.newDeploy(factory, curl("cs:quantal/hive-23"))
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Prepare(operation.State{})
		c.Check(newState, gc.IsNil)
		c.Check(err, gc.ErrorMatches, "blargh")
		c.Check(callbacks.MockSetCurrentCharm.gotCharmURL, gc.DeepEquals, curl("cs:quantal/hive-23"))
	}
}

func (s *DeploySuite) TestPrepareSuccess(c *gc.C) {
	var stateChangeTests = []struct {
		description string
		kind        operation.Kind
		before      operation.State
		after       operation.State
	}{{
		description: "sets kind/step/url over blank state",
		kind:        operation.Install,
		after: operation.State{
			Kind:     operation.Install,
			Step:     operation.Pending,
			CharmURL: curl("cs:quantal/nyancat-4"),
		},
	}, {
		description: "sets kind/step/url over queued install",
		kind:        operation.Install,
		before: operation.State{
			Kind:     operation.Install,
			Step:     operation.Queued,
			CharmURL: curl("cs:quantal/nyancat-4"),
		},
		after: operation.State{
			Kind:     operation.Install,
			Step:     operation.Pending,
			CharmURL: curl("cs:quantal/nyancat-4"),
		},
	}, {
		description: "preserves hook when pending RunHook interrupted",
		kind:        operation.Upgrade,
		before: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
		after: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Pending,
			CharmURL: curl("cs:quantal/nyancat-4"),
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
		},
	}, {
		description: "preserves hook when Upgrade interrupted",
		kind:        operation.Upgrade,
		before: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Pending,
			CharmURL: curl("cs:quantal/random-23"),
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
		},
		after: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Pending,
			CharmURL: curl("cs:quantal/nyancat-4"),
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
		},
	}, {
		description: "drops hook in other situations, preserves started/metrics",
		kind:        operation.Upgrade,
		before:      overwriteState,
		after: operation.State{
			Kind:               operation.Upgrade,
			Step:               operation.Pending,
			CharmURL:           curl("cs:quantal/nyancat-4"),
			Started:            true,
			CollectMetricsTime: 1234567,
		},
	}}

	for i, test := range stateChangeTests {
		c.Logf("test %d: %s", i, test.description)

		for j, info := range allDeployInfo {
			c.Logf("variant %d", j)
			if info.expectKind != test.kind {
				continue
			}
			callbacks := NewDeployCallbacks()
			deployer := &MockDeployer{
				MockNotifyRevert:   &MockNoArgs{},
				MockNotifyResolved: &MockNoArgs{},
				MockStage:          &MockStage{},
			}
			factory := operation.NewFactory(deployer, nil, callbacks, nil)
			op, err := info.newDeploy(factory, curl("cs:quantal/nyancat-4"))
			c.Assert(err, jc.ErrorIsNil)

			newState, err := op.Prepare(test.before)
			c.Check(err, jc.ErrorIsNil)
			c.Check(newState, gc.DeepEquals, &test.after)
			c.Check(callbacks.MockSetCurrentCharm.gotCharmURL, gc.DeepEquals, curl("cs:quantal/nyancat-4"))
		}
	}
}

func (s *DeploySuite) TestExecuteError(c *gc.C) {
	for i, info := range allDeployInfo {
		c.Logf("test %d", i)
		callbacks := NewDeployCallbacks()
		deployer := &MockDeployer{
			MockNotifyRevert:   &MockNoArgs{},
			MockNotifyResolved: &MockNoArgs{},
			MockStage:          &MockStage{},
			MockDeploy:         &MockNoArgs{err: errors.New("rasp")},
		}
		factory := operation.NewFactory(deployer, nil, callbacks, nil)
		op, err := info.newDeploy(factory, curl("cs:quantal/nyancat-4"))
		c.Assert(err, jc.ErrorIsNil)
		_, err = op.Prepare(operation.State{})
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Execute(operation.State{})
		c.Check(newState, gc.IsNil)
		c.Check(err, gc.ErrorMatches, "rasp")
		c.Check(deployer.MockDeploy.called, jc.IsTrue)
	}
}

func (s *DeploySuite) TestExecuteSuccess(c *gc.C) {
	var stateChangeTests = []struct {
		description string
		kind        operation.Kind
		before      operation.State
		after       operation.State
	}{{
		description: "install over blank slate",
		kind:        operation.Install,
		after: operation.State{
			Kind:     operation.Install,
			Step:     operation.Done,
			CharmURL: curl("cs:quantal/lol-1"),
		},
	}, {
		description: "install queued",
		kind:        operation.Install,
		before: operation.State{
			Kind:     operation.Install,
			Step:     operation.Queued,
			CharmURL: curl("cs:quantal/lol-1"),
		},
		after: operation.State{
			Kind:     operation.Install,
			Step:     operation.Done,
			CharmURL: curl("cs:quantal/lol-1"),
		},
	}, {
		description: "upgrade after hook",
		kind:        operation.Upgrade,
		before: operation.State{
			Kind: operation.Continue,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
		after: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Done,
			CharmURL: curl("cs:quantal/lol-1"),
		},
	}, {
		description: "upgrade interrupts hook",
		kind:        operation.Upgrade,
		before: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
		after: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Done,
			CharmURL: curl("cs:quantal/lol-1"),
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
		},
	}, {
		description: "upgrade preserves started/metrics",
		kind:        operation.Upgrade,
		before:      overwriteState,
		after: operation.State{
			Kind:               operation.Upgrade,
			Step:               operation.Done,
			CharmURL:           curl("cs:quantal/lol-1"),
			Started:            true,
			CollectMetricsTime: 1234567,
		},
	}}

	for i, test := range stateChangeTests {
		c.Logf("test %d: %s", i, test.description)
		for j, info := range allDeployInfo {
			c.Logf("variant %d", j)
			if test.kind != info.expectKind {
				continue
			}
			deployer := NewMockDeployer()
			callbacks := NewDeployCallbacks()
			factory := operation.NewFactory(deployer, nil, callbacks, nil)
			op, err := info.newDeploy(factory, curl("cs:quantal/lol-1"))
			c.Assert(err, jc.ErrorIsNil)

			midState, err := op.Prepare(test.before)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(midState, gc.NotNil)

			newState, err := op.Execute(*midState)
			c.Check(err, jc.ErrorIsNil)
			c.Check(newState, gc.DeepEquals, &test.after)
			c.Check(deployer.MockDeploy.called, jc.IsTrue)
		}
	}
}

func (s *DeploySuite) TestCommitMetricsError(c *gc.C) {
	for i, info := range allDeployInfo {
		c.Logf("test %d", i)
		callbacks := NewDeployCommitCallbacks(errors.New("glukh"))
		factory := operation.NewFactory(nil, nil, callbacks, nil)
		op, err := info.newDeploy(factory, curl("cs:quantal/x-0"))
		c.Assert(err, jc.ErrorIsNil)
		newState, err := op.Commit(operation.State{})
		c.Check(err, gc.ErrorMatches, "glukh")
		c.Check(newState, gc.IsNil)
	}
}

func (s *DeploySuite) TestCommitQueueInstallHook(c *gc.C) {
	callbacks := NewDeployCommitCallbacks(nil)
	factory := operation.NewFactory(nil, nil, callbacks, nil)
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

func (s *DeploySuite) TestCommitQueueUpgradeHook(c *gc.C) {
	for i, info := range allDeployInfo {
		c.Logf("test %d", i)
		if info.expectKind != operation.Upgrade {
			continue
		}
		callbacks := NewDeployCommitCallbacks(nil)
		factory := operation.NewFactory(nil, nil, callbacks, nil)
		op, err := info.newDeploy(factory, curl("cs:quantal/x-0"))
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
}

func (s *DeploySuite) TestCommitInterruptedHook(c *gc.C) {
	for i, info := range allDeployInfo {
		c.Logf("test %d", i)
		if info.expectKind != operation.Upgrade {
			continue
		}
		callbacks := NewDeployCommitCallbacks(nil)
		factory := operation.NewFactory(nil, nil, callbacks, nil)
		op, err := info.newDeploy(factory, curl("cs:quantal/x-0"))
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
}
