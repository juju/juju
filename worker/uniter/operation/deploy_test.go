// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type DeploySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DeploySuite{})

var bothDeployKinds = []operation.Kind{operation.Install, operation.Upgrade}

func (s *DeploySuite) TestPrepareAlreadyDone(c *gc.C) {
	for _, kind := range bothDeployKinds {
		factory := operation.NewFactory(nil, nil, nil, nil)
		c.Logf("testing %s", kind)
		op, err := factory.NewDeploy(curl("cs:quantal/hive-23"), kind)
		c.Assert(err, jc.ErrorIsNil)
		newState, err := op.Prepare(operation.State{
			Kind:     kind,
			Step:     operation.Done,
			CharmURL: curl("cs:quantal/hive-23"),
		})
		c.Assert(newState, gc.IsNil)
		c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	}
}

func (s *DeploySuite) TestPrepareArchiveInfoError(c *gc.C) {
	for _, kind := range bothDeployKinds {
		c.Logf("testing %s", kind)
		callbacks := &DeployCallbacks{
			MockGetArchiveInfo: &MockGetArchiveInfo{err: errors.New("pew")},
		}
		factory := operation.NewFactory(nil, nil, callbacks, nil)
		op, err := factory.NewDeploy(curl("cs:quantal/hive-23"), kind)
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Prepare(operation.State{})
		c.Assert(newState, gc.IsNil)
		c.Assert(err, gc.ErrorMatches, "pew")
		c.Assert(callbacks.MockGetArchiveInfo.gotCharmURL, gc.DeepEquals, curl("cs:quantal/hive-23"))
	}
}

func (s *DeploySuite) TestPrepareStageError(c *gc.C) {
	for _, kind := range bothDeployKinds {
		c.Logf("testing %s", kind)
		callbacks := &DeployCallbacks{
			MockGetArchiveInfo: &MockGetArchiveInfo{info: &MockBundleInfo{}},
		}
		deployer := &MockDeployer{
			MockStage: &MockStage{err: errors.New("squish")},
		}
		var abort <-chan struct{} = make(chan struct{})
		factory := operation.NewFactory(deployer, nil, callbacks, abort)
		op, err := factory.NewDeploy(curl("cs:quantal/hive-23"), kind)
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Prepare(operation.State{})
		c.Assert(newState, gc.IsNil)
		c.Assert(err, gc.ErrorMatches, "squish")
		c.Assert(*deployer.MockStage.gotInfo, gc.Equals, callbacks.MockGetArchiveInfo.info)
		c.Assert(*deployer.MockStage.gotAbort, gc.Equals, abort)
	}
}

func (s *DeploySuite) TestPrepareSetCharmError(c *gc.C) {
	for _, kind := range bothDeployKinds {
		c.Logf("testing %s", kind)
		callbacks := &DeployCallbacks{
			MockGetArchiveInfo:  &MockGetArchiveInfo{},
			MockSetCurrentCharm: &MockSetCurrentCharm{err: errors.New("blargh")},
		}
		deployer := &MockDeployer{MockStage: &MockStage{}}
		factory := operation.NewFactory(deployer, nil, callbacks, nil)
		op, err := factory.NewDeploy(curl("cs:quantal/hive-23"), kind)
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Prepare(operation.State{})
		c.Assert(newState, gc.IsNil)
		c.Assert(err, gc.ErrorMatches, "blargh")
		c.Assert(callbacks.MockSetCurrentCharm.gotCharmURL, gc.DeepEquals, curl("cs:quantal/hive-23"))
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
		callbacks := NewPrepareDeploySuccessCallbacks()
		deployer := &MockDeployer{MockStage: &MockStage{}}
		factory := operation.NewFactory(deployer, nil, callbacks, nil)
		op, err := factory.NewDeploy(curl("cs:quantal/nyancat-4"), test.kind)
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Prepare(test.before)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(newState, gc.NotNil)
		c.Check(*newState, gc.DeepEquals, test.after)
		c.Assert(callbacks.MockSetCurrentCharm.gotCharmURL, gc.DeepEquals, curl("cs:quantal/nyancat-4"))
	}
}

func (s *DeploySuite) TestExecuteError(c *gc.C) {
	for _, kind := range bothDeployKinds {
		c.Logf("testing %s", kind)
		callbacks := NewPrepareDeploySuccessCallbacks()
		deployer := &MockDeployer{
			MockStage:  &MockStage{},
			MockDeploy: &MockDeploy{err: errors.New("rasp")},
		}
		factory := operation.NewFactory(deployer, nil, callbacks, nil)
		op, err := factory.NewDeploy(curl("cs:quantal/nyancat-4"), kind)
		c.Assert(err, jc.ErrorIsNil)
		_, err = op.Prepare(operation.State{})
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Execute(operation.State{})
		c.Assert(newState, gc.IsNil)
		c.Assert(err, gc.ErrorMatches, "rasp")
		c.Assert(deployer.MockDeploy.called, jc.IsTrue)
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
		callbacks, deployer := NewDeployExecuteSuccessFixture()
		factory := operation.NewFactory(deployer, nil, callbacks, nil)
		op, err := factory.NewDeploy(curl("cs:quantal/lol-1"), test.kind)
		c.Assert(err, jc.ErrorIsNil)

		midState, err := op.Prepare(test.before)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(midState, gc.NotNil)

		newState, err := op.Execute(*midState)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(newState, gc.NotNil)
		c.Assert(*newState, gc.DeepEquals, test.after)
		c.Assert(deployer.MockDeploy.called, jc.IsTrue)
	}
}

func (s *DeploySuite) TestCommitQueueInstallHook(c *gc.C) {
	factory := operation.NewFactory(nil, nil, nil, nil)
	op, err := factory.NewDeploy(curl("cs:quantal/x-0"), operation.Install)
	c.Assert(err, jc.ErrorIsNil)
	newState, err := op.Commit(operation.State{
		Kind:     operation.Install,
		Step:     operation.Done,
		CharmURL: nil, // doesn't actually matter here
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.NotNil)
	c.Assert(*newState, gc.DeepEquals, operation.State{
		Kind: operation.RunHook,
		Step: operation.Queued,
		Hook: &hook.Info{Kind: hooks.Install},
	})
}

func (s *DeploySuite) TestCommitQueueUpgradeHook(c *gc.C) {
	factory := operation.NewFactory(nil, nil, nil, nil)
	op, err := factory.NewDeploy(curl("cs:quantal/x-0"), operation.Upgrade)
	c.Assert(err, jc.ErrorIsNil)
	newState, err := op.Commit(operation.State{
		Kind:     operation.Upgrade,
		Step:     operation.Done,
		CharmURL: nil, // doesn't actually matter here
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.NotNil)
	c.Assert(*newState, gc.DeepEquals, operation.State{
		Kind: operation.RunHook,
		Step: operation.Queued,
		Hook: &hook.Info{Kind: hooks.UpgradeCharm},
	})
}

func (s *DeploySuite) TestCommitInterruptedHook(c *gc.C) {
	factory := operation.NewFactory(nil, nil, nil, nil)
	op, err := factory.NewDeploy(curl("cs:quantal/x-0"), operation.Upgrade)
	c.Assert(err, jc.ErrorIsNil)
	newState, err := op.Commit(operation.State{
		Kind:     operation.Upgrade,
		Step:     operation.Done,
		CharmURL: nil, // doesn't actually matter here
		Hook:     &hook.Info{Kind: hooks.ConfigChanged},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.NotNil)
	c.Assert(*newState, gc.DeepEquals, operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{Kind: hooks.ConfigChanged},
	})
}
