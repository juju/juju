// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type DeploySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DeploySuite{})

func (s *DeploySuite) TestPrepareAlreadyDone(c *gc.C) {
	factory := operation.NewFactory(nil, nil, nil, nil)

	op, err := factory.NewDeploy(curl("cs:quantal/hive-23"), operation.Install)
	c.Assert(err, gc.IsNil)
	newState, err := op.Prepare(operation.State{
		Kind:     operation.Install,
		Step:     operation.Done,
		CharmURL: curl("cs:quantal/hive-23"),
	})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)

	op, err = factory.NewDeploy(curl("cs:quantal/hive-23"), operation.Upgrade)
	c.Assert(err, gc.IsNil)
	newState, err = op.Prepare(operation.State{
		Kind:     operation.Upgrade,
		Step:     operation.Done,
		CharmURL: curl("cs:quantal/hive-23"),
	})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
}

func (s *DeploySuite) TestPrepareArchiveInfoError(c *gc.C) {
	callbacks := &DeployCallbacks{
		MockGetArchiveInfo: &MockGetArchiveInfo{err: errors.New("pew")},
	}
	factory := operation.NewFactory(nil, nil, callbacks, nil)
	op, err := factory.NewDeploy(curl("cs:quantal/hive-23"), operation.Upgrade)
	c.Assert(err, gc.IsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "pew")
	c.Assert(callbacks.MockGetArchiveInfo.gotCharmURL, gc.DeepEquals, curl("cs:quantal/hive-23"))
}

func (s *DeploySuite) TestPrepareStageError(c *gc.C) {
	callbacks := &DeployCallbacks{
		MockGetArchiveInfo: &MockGetArchiveInfo{info: &MockBundleInfo{}},
	}
	deployer := &MockDeployer{
		MockStage: &MockStage{err: errors.New("squish")},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(deployer, nil, callbacks, abort)
	op, err := factory.NewDeploy(curl("cs:quantal/hive-23"), operation.Upgrade)
	c.Assert(err, gc.IsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "squish")
	c.Assert(*deployer.MockStage.gotInfo, gc.Equals, callbacks.MockGetArchiveInfo.info)
	c.Assert(*deployer.MockStage.gotAbort, gc.Equals, abort)
}

func (s *DeploySuite) TestPrepareSetCharmError(c *gc.C) {
	callbacks := &DeployCallbacks{
		MockGetArchiveInfo:  &MockGetArchiveInfo{},
		MockSetCurrentCharm: &MockSetCurrentCharm{err: errors.New("blargh")},
	}
	deployer := &MockDeployer{MockStage: &MockStage{}}
	factory := operation.NewFactory(deployer, nil, callbacks, nil)
	op, err := factory.NewDeploy(curl("cs:quantal/hive-23"), operation.Install)
	c.Assert(err, gc.IsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "blargh")
	c.Assert(callbacks.MockSetCurrentCharm.gotCharmURL, gc.DeepEquals, curl("cs:quantal/hive-23"))
}

func (s *DeploySuite) TestPrepareSuccess(c *gc.C) {
	var stateChangeTests = []struct {
		description string
		before      operation.State
		after       operation.State
	}{{
		description: "sets kind/step/url over blank state",
		after: operation.State{
			Step:     operation.Pending,
			CharmURL: curl("cs:quantal/nyancat-4"),
		},
	}, {
		description: "preserves hook when pending RunHook interrupted",
		before: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
		after: operation.State{
			Step:     operation.Pending,
			CharmURL: curl("cs:quantal/nyancat-4"),
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
		},
	}, {
		description: "preserves hook when Upgrade interrupted",
		before: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Pending,
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			CharmURL: curl("cs:quantal/random-23"),
		},
		after: operation.State{
			Step:     operation.Pending,
			CharmURL: curl("cs:quantal/nyancat-4"),
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
		},
	}, {
		description: "drops hook in other situations, preserves started/metrics",
		before:      overwriteState,
		after: operation.State{
			Started:            true,
			CollectMetricsTime: 1234567,
			Step:               operation.Pending,
			CharmURL:           curl("cs:quantal/nyancat-4"),
		},
	}}

	for _, kind := range []operation.Kind{operation.Install, operation.Upgrade} {
		c.Logf("testing %s", kind)
		for i, test := range stateChangeTests {
			c.Logf("test %d: %s", i, test.description)
			callbacks := NewPrepareDeploySuccessCallbacks()
			deployer := &MockDeployer{MockStage: &MockStage{}}
			factory := operation.NewFactory(deployer, nil, callbacks, nil)
			op, err := factory.NewDeploy(curl("cs:quantal/nyancat-4"), kind)
			c.Assert(err, gc.IsNil)

			newState, err := op.Prepare(test.before)
			c.Assert(err, gc.IsNil)
			c.Assert(newState, gc.NotNil)
			expectState := test.after
			expectState.Kind = kind
			c.Check(*newState, gc.DeepEquals, expectState)
		}
	}
}

func (s *DeploySuite) TestExecuteError(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *DeploySuite) TestExecuteSuccess(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *DeploySuite) TestCommitQueueInstallHook(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *DeploySuite) TestCommitQueueUpgradeHook(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *DeploySuite) TestCommitInterruptedHook(c *gc.C) {
	c.Fatalf("XXX")
}
