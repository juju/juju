// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver_test

import (
	"context"
	"errors"
	"time"

	"github.com/juju/mutex/v2"
	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/testcharms"
)

type LoopSuite struct {
	testing.BaseSuite

	resolver  resolver.Resolver
	watcher   *mockRemoteStateWatcher
	opFactory *mockOpFactory
	executor  *mockOpExecutor
	charmURL  string
	charmDir  string
	abort     chan struct{}
	onIdle    func() error
}

var _ = gc.Suite(&LoopSuite{})

func (s *LoopSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resolver = resolver.ResolverFunc(func(context.Context, resolver.LocalState, remotestate.Snapshot, operation.Factory) (operation.Operation, error) {
		return nil, resolver.ErrNoOperation
	})
	s.watcher = &mockRemoteStateWatcher{
		changes: make(chan struct{}, 1),
	}
	s.opFactory = &mockOpFactory{}
	s.executor = &mockOpExecutor{}
	s.charmURL = "ch:trusty/mysql-1"
	s.abort = make(chan struct{})
}

func (s *LoopSuite) loop(c *gc.C) (resolver.LocalState, error) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
	}
	err := resolver.Loop(context.Background(), resolver.LoopConfig{
		Resolver:      s.resolver,
		Factory:       s.opFactory,
		Watcher:       s.watcher,
		Executor:      s.executor,
		OnIdle:        s.onIdle,
		CharmDir:      s.charmDir,
		CharmDirGuard: &mockCharmDirGuard{},
		Logger:        loggertesting.WrapCheckLog(c),
	}, &localState)
	return localState, err
}

func (s *LoopSuite) TestAbort(c *gc.C) {
	close(s.abort)
	_, err := s.loop(c)
	c.Assert(err, gc.Equals, resolver.ErrLoopAborted)
}

func (s *LoopSuite) TestOnIdle(c *gc.C) {
	onIdleCh := make(chan interface{}, 1)
	s.onIdle = func() error {
		onIdleCh <- nil
		return nil
	}

	done := make(chan interface{}, 1)
	go func() {
		_, err := s.loop(c)
		done <- err
	}()

	waitChannel(c, onIdleCh, "waiting for onIdle")
	s.watcher.changes <- struct{}{}
	waitChannel(c, onIdleCh, "waiting for onIdle")
	close(s.abort)

	err := waitChannel(c, done, "waiting for loop to exit")
	c.Assert(err, gc.Equals, resolver.ErrLoopAborted)

	select {
	case <-onIdleCh:
		c.Fatal("unexpected onIdle call")
	default:
	}
}

func (s *LoopSuite) TestOnIdleError(c *gc.C) {
	s.onIdle = func() error {
		return errors.New("onIdle failed")
	}
	close(s.abort)
	_, err := s.loop(c)
	c.Assert(err, gc.ErrorMatches, "onIdle failed")
}

func (s *LoopSuite) TestErrWaitingNoOnIdle(c *gc.C) {
	var onIdleCalled bool
	s.onIdle = func() error {
		onIdleCalled = true
		return nil
	}
	s.resolver = resolver.ResolverFunc(func(
		_ context.Context,
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		return nil, resolver.ErrWaiting
	})
	close(s.abort)
	_, err := s.loop(c)
	c.Assert(err, gc.Equals, resolver.ErrLoopAborted)
	c.Assert(onIdleCalled, jc.IsFalse)
}

func (s *LoopSuite) TestInitialFinalLocalState(c *gc.C) {
	var local resolver.LocalState
	s.resolver = resolver.ResolverFunc(func(
		_ context.Context,
		l resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		local = l
		return nil, resolver.ErrNoOperation
	})

	close(s.abort)
	lastLocal, err := s.loop(c)
	c.Assert(err, gc.Equals, resolver.ErrLoopAborted)
	c.Assert(local, jc.DeepEquals, resolver.LocalState{
		CharmURL: s.charmURL,
	})
	c.Assert(lastLocal, jc.DeepEquals, local)
}

func (s *LoopSuite) TestLoop(c *gc.C) {
	var resolverCalls int
	theOp := &mockOp{}
	s.resolver = resolver.ResolverFunc(func(
		_ context.Context,
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		resolverCalls++
		switch resolverCalls {
		// On the first call, return an operation.
		case 1:
			return theOp, nil
		// On the second call, simulate having
		// no operations to perform, at which
		// point we'll wait for a remote state
		// change.
		case 2:
			s.watcher.changes <- struct{}{}
			break
		// On the third call, kill the loop.
		case 3:
			close(s.abort)
			break
		}
		return nil, resolver.ErrNoOperation
	})

	_, err := s.loop(c)
	c.Assert(err, gc.Equals, resolver.ErrLoopAborted)
	c.Assert(resolverCalls, gc.Equals, 3)
	s.executor.CheckCallNames(c, "State", "State", "State", "Run", "State", "State")

	runArgs := s.executor.Calls()[3].Args
	c.Assert(runArgs, gc.HasLen, 2)
	c.Assert(runArgs[0], gc.DeepEquals, theOp)
	c.Assert(runArgs[1], gc.NotNil)
}

func (s *LoopSuite) TestLoopWithChange(c *gc.C) {
	var resolverCalls int
	theOp := &mockOp{}
	s.resolver = resolver.ResolverFunc(func(
		_ context.Context,
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		resolverCalls++
		switch resolverCalls {
		// On the first call, return an operation.
		case 1:
			return theOp, nil
		// On the second call, simulate having
		// no operations to perform, at which
		// point we'll wait for a remote state
		// change.
		case 2:
			s.watcher.changes <- struct{}{}
			break
		case 3:
			break
		// On the fourth call, kill the loop.
		case 4:
			close(s.abort)
			break
		}
		return nil, resolver.ErrNoOperation
	})

	var remoteStateSnapshotChan <-chan remotestate.Snapshot
	remoteStateSnapshotCount := 0
	s.executor.run = func(op operation.Operation, rs <-chan remotestate.Snapshot) error {
		remoteStateSnapshotChan = rs
		for i := 0; i < 5; i++ {
			// queue up a change to trigger snapshot channel.
			s.watcher.changes <- struct{}{}
			// wait for changes to propagate
			select {
			case _, ok := <-rs:
				c.Assert(ok, jc.IsTrue)
				remoteStateSnapshotCount++
			case <-time.After(testing.ShortWait):
				c.Fatalf("timed out waiting for remote state snapshot")
				panic("unreachable")
			}
		}
		return nil
	}

	_, err := s.loop(c)
	c.Assert(err, gc.Equals, resolver.ErrLoopAborted)
	c.Assert(resolverCalls, gc.Equals, 4)
	s.executor.CheckCallNames(c, "State", "State", "State", "Run", "State", "State", "State")

	c.Assert(remoteStateSnapshotCount, gc.Equals, 5)
	select {
	case _, ok := <-remoteStateSnapshotChan:
		c.Assert(ok, jc.IsTrue)
		c.Fatalf("remote state snapshot channel fired more than once")
	default:
	}

	runArgs := s.executor.Calls()[3].Args
	c.Assert(runArgs, gc.HasLen, 2)
	c.Assert(runArgs[0], gc.DeepEquals, theOp)
	c.Assert(runArgs[1], gc.NotNil)
}

func (s *LoopSuite) TestRunFails(c *gc.C) {
	s.executor.SetErrors(errors.New("run fails"))
	s.resolver = resolver.ResolverFunc(func(
		_ context.Context,
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		return mockOp{}, nil
	})
	_, err := s.loop(c)
	c.Assert(err, gc.ErrorMatches, "run fails")
}

func (s *LoopSuite) TestNextOpFails(c *gc.C) {
	s.resolver = resolver.ResolverFunc(func(
		_ context.Context,
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		return nil, errors.New("NextOp fails")
	})
	_, err := s.loop(c)
	c.Assert(err, gc.ErrorMatches, "NextOp fails")
}

func (s *LoopSuite) TestCheckCharmUpgradeUpgradeCharmHook(c *gc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     envtesting.Stub{},
		st: operation.State{
			Installed: true,
			Kind:      operation.Continue,
			Hook:      &hook.Info{Kind: hooks.UpgradeCharm},
		},
		run: nil,
	}
	s.testCheckCharmUpgradeDoesNothing(c)
}

func (s *LoopSuite) TestCheckCharmUpgradeSameURL(c *gc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     envtesting.Stub{},
		st: operation.State{
			Installed: true,
			Kind:      operation.Continue,
		},
		run: nil,
	}
	s.watcher = &mockRemoteStateWatcher{
		snapshot: remotestate.Snapshot{
			CharmURL: s.charmURL,
		},
	}
	s.charmDir = testcharms.Repo.CharmDirPath("mysql")
	s.testCheckCharmUpgradeDoesNothing(c)
}

func (s *LoopSuite) TestCheckCharmUpgradeNotInstalled(c *gc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     envtesting.Stub{},
		st: operation.State{
			Kind: operation.Continue,
		},
		run: nil,
	}
	s.watcher = &mockRemoteStateWatcher{
		snapshot: remotestate.Snapshot{
			CharmURL: "ch:trusty/mysql-2",
		},
	}
	s.charmDir = testcharms.Repo.CharmDirPath("mysql")
	s.testCheckCharmUpgradeDoesNothing(c)
}

func (s *LoopSuite) TestCheckCharmUpgradeIncorrectLXDProfile(c *gc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     envtesting.Stub{},
		st: operation.State{
			Installed: true,
			Started:   true,
			Kind:      operation.Continue,
		},
		run: nil,
	}
	s.watcher = &mockRemoteStateWatcher{
		snapshot: remotestate.Snapshot{
			CharmURL:             "ch:trusty/mysql-2",
			CharmProfileRequired: true,
			LXDProfileName:       "juju-test-mysql-1",
		},
	}
	s.testCheckCharmUpgradeDoesNothing(c)
}

func (s *LoopSuite) testCheckCharmUpgradeDoesNothing(c *gc.C) {
	s.resolver = resolver.ResolverFunc(func(
		_ context.Context,
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		return nil, resolver.ErrWaiting
	})
	close(s.abort)
	_, err := s.loop(c)
	c.Assert(err, gc.Equals, resolver.ErrLoopAborted)

	// Run not called
	c.Assert(s.executor.Calls(), gc.HasLen, 3)
	s.executor.CheckCallNames(c, "State", "State", "State")
}

func (s *LoopSuite) TestCheckCharmUpgrade(c *gc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     envtesting.Stub{},
		st: operation.State{
			Installed: true,
			Kind:      operation.Continue,
		},
		run: nil,
	}
	s.watcher = &mockRemoteStateWatcher{
		snapshot: remotestate.Snapshot{
			CharmURL: "ch:trusty/mysql-2",
		},
	}
	s.testCheckCharmUpgradeCallsRun(c, "Upgrade")
}

func (s *LoopSuite) TestCheckCharmUpgradeMissingCharmDir(c *gc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     envtesting.Stub{},
		st: operation.State{
			Installed: true,
			Kind:      operation.Continue,
		},
		run: nil,
	}
	s.watcher = &mockRemoteStateWatcher{
		snapshot: remotestate.Snapshot{
			CharmURL: s.charmURL,
		},
	}
	s.testCheckCharmUpgradeCallsRun(c, "Upgrade")
}

func (s *LoopSuite) TestCheckCharmInstallMissingCharmDirInstallHookFail(c *gc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     envtesting.Stub{},
		st: operation.State{
			Installed: false,
			Kind:      operation.RunHook,
			Step:      operation.Pending,
			Hook:      &hook.Info{Kind: hooks.Install},
		},
		run: nil,
	}
	s.watcher = &mockRemoteStateWatcher{
		snapshot: remotestate.Snapshot{
			CharmURL: s.charmURL,
		},
	}
	s.testCheckCharmUpgradeCallsRun(c, "Install")
}

func (s *LoopSuite) TestCheckCharmUpgradeLXDProfile(c *gc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     envtesting.Stub{},
		st: operation.State{
			Installed: true,
			Started:   true,
			Kind:      operation.Continue,
		},
		run: nil,
	}
	s.watcher = &mockRemoteStateWatcher{
		snapshot: remotestate.Snapshot{
			CharmURL:             "ch:trusty/mysql-2",
			CharmProfileRequired: true,
			LXDProfileName:       "juju-test-mysql-2",
		},
	}
	s.testCheckCharmUpgradeCallsRun(c, "Upgrade")
}

func (s *LoopSuite) testCheckCharmUpgradeCallsRun(c *gc.C, op string) {
	s.opFactory = &mockOpFactory{
		Factory: nil,
		Stub:    envtesting.Stub{},
		op:      mockOp{},
	}
	s.resolver = resolver.ResolverFunc(func(
		_ context.Context,
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		return nil, resolver.ErrWaiting
	})
	close(s.abort)
	_, err := s.loop(c)
	c.Assert(err, gc.Equals, resolver.ErrLoopAborted)

	// Run not called
	c.Assert(s.executor.Calls(), gc.HasLen, 4)
	s.executor.CheckCallNames(c, "State", "State", "Run", "State")

	c.Assert(s.opFactory.Calls(), gc.HasLen, 1)
	s.opFactory.CheckCallNames(c, "New"+op)
}

func (s *LoopSuite) TestCancelledLockAcquisitionCausesRestart(c *gc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     envtesting.Stub{},
		st: operation.State{
			Started: true,
			Kind:    operation.Continue,
		},
		run: func(operation.Operation, <-chan remotestate.Snapshot) error {
			return mutex.ErrCancelled
		},
	}

	s.resolver = resolver.ResolverFunc(func(
		_ context.Context,
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		return &mockOp{}, nil
	})

	_, err := s.loop(c)
	c.Assert(err, gc.Equals, resolver.ErrRestart)
}

func waitChannel(c *gc.C, ch <-chan interface{}, activity string) interface{} {
	select {
	case v := <-ch:
		return v
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out %s", activity)
		panic("unreachable")
	}
}
