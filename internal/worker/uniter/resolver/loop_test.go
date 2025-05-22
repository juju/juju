// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver_test

import (
	"context"
	"errors"
	stdtesting "testing"
	"time"

	"github.com/juju/mutex/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
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
	onIdle    func() error
}

func TestLoopSuite(t *stdtesting.T) {
	tc.Run(t, &LoopSuite{})
}

func (s *LoopSuite) SetUpTest(c *tc.C) {
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
}

func (s *LoopSuite) loop(c *tc.C) func(context.Context) (resolver.LocalState, error) {
	return func(ctx context.Context) (resolver.LocalState, error) {
		localState := resolver.LocalState{
			CharmURL: s.charmURL,
		}
		err := resolver.Loop(ctx, resolver.LoopConfig{
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
}

func (s *LoopSuite) TestAbort(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	_, err := s.loop(c)(ctx)
	c.Assert(err, tc.Equals, resolver.ErrLoopAborted)
}

func (s *LoopSuite) TestOnIdle(c *tc.C) {
	onIdleCh := make(chan interface{}, 1)
	s.onIdle = func() error {
		onIdleCh <- nil
		return nil
	}

	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	loopFn := s.loop(c)

	done := make(chan interface{}, 1)
	go func() {
		_, err := loopFn(ctx)
		done <- err
	}()

	waitChannel(c, onIdleCh, "waiting for onIdle")
	s.watcher.changes <- struct{}{}
	waitChannel(c, onIdleCh, "waiting for onIdle")
	cancel()

	err := waitChannel(c, done, "waiting for loop to exit")
	c.Assert(err, tc.Equals, resolver.ErrLoopAborted)

	select {
	case <-onIdleCh:
		c.Fatal("unexpected onIdle call")
	default:
	}
}

func (s *LoopSuite) TestOnIdleError(c *tc.C) {
	s.onIdle = func() error {
		return errors.New("onIdle failed")
	}

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	_, err := s.loop(c)(ctx)
	c.Assert(err, tc.ErrorMatches, "onIdle failed")
}

func (s *LoopSuite) TestErrWaitingNoOnIdle(c *tc.C) {
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

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	_, err := s.loop(c)(ctx)
	c.Assert(err, tc.Equals, resolver.ErrLoopAborted)
	c.Assert(onIdleCalled, tc.IsFalse)
}

func (s *LoopSuite) TestInitialFinalLocalState(c *tc.C) {
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

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	lastLocal, err := s.loop(c)(ctx)
	c.Assert(err, tc.Equals, resolver.ErrLoopAborted)
	c.Assert(local, tc.DeepEquals, resolver.LocalState{
		CharmURL: s.charmURL,
	})
	c.Assert(lastLocal, tc.DeepEquals, local)
}

func (s *LoopSuite) TestLoop(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

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
		// On the third call, kill the loop.
		case 3:
			cancel()
		}
		return nil, resolver.ErrNoOperation
	})

	_, err := s.loop(c)(ctx)
	c.Assert(err, tc.Equals, resolver.ErrLoopAborted)
	c.Assert(resolverCalls, tc.Equals, 3)
	s.executor.CheckCallNames(c, "State", "State", "State", "Run", "State", "State")

	runArgs := s.executor.Calls()[3].Args
	c.Assert(runArgs, tc.HasLen, 2)
	c.Assert(runArgs[0], tc.DeepEquals, theOp)
	c.Assert(runArgs[1], tc.NotNil)
}

func (s *LoopSuite) TestLoopWithChange(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

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
		case 3:
		// On the fourth call, kill the loop.
		case 4:
			cancel()
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
				c.Assert(ok, tc.IsTrue)
				remoteStateSnapshotCount++
			case <-time.After(testing.ShortWait):
				c.Fatalf("timed out waiting for remote state snapshot")
				panic("unreachable")
			}
		}
		return nil
	}

	_, err := s.loop(c)(ctx)
	c.Assert(err, tc.Equals, resolver.ErrLoopAborted)
	c.Assert(resolverCalls, tc.Equals, 4)
	s.executor.CheckCallNames(c, "State", "State", "State", "Run", "State", "State", "State")

	c.Assert(remoteStateSnapshotCount, tc.Equals, 5)
	select {
	case _, ok := <-remoteStateSnapshotChan:
		c.Assert(ok, tc.IsTrue)
		c.Fatalf("remote state snapshot channel fired more than once")
	default:
	}

	runArgs := s.executor.Calls()[3].Args
	c.Assert(runArgs, tc.HasLen, 2)
	c.Assert(runArgs[0], tc.DeepEquals, theOp)
	c.Assert(runArgs[1], tc.NotNil)
}

func (s *LoopSuite) TestRunFails(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	s.executor.SetErrors(errors.New("run fails"))
	s.resolver = resolver.ResolverFunc(func(
		_ context.Context,
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		return mockOp{}, nil
	})
	_, err := s.loop(c)(ctx)
	c.Assert(err, tc.ErrorMatches, "run fails")
}

func (s *LoopSuite) TestNextOpFails(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	s.resolver = resolver.ResolverFunc(func(
		_ context.Context,
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		return nil, errors.New("NextOp fails")
	})
	_, err := s.loop(c)(ctx)
	c.Assert(err, tc.ErrorMatches, "NextOp fails")
}

func (s *LoopSuite) TestCheckCharmUpgradeUpgradeCharmHook(c *tc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     testhelpers.Stub{},
		st: operation.State{
			Installed: true,
			Kind:      operation.Continue,
			Hook:      &hook.Info{Kind: hooks.UpgradeCharm},
		},
		run: nil,
	}
	s.testCheckCharmUpgradeDoesNothing(c)
}

func (s *LoopSuite) TestCheckCharmUpgradeSameURL(c *tc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     testhelpers.Stub{},
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

func (s *LoopSuite) TestCheckCharmUpgradeNotInstalled(c *tc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     testhelpers.Stub{},
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

func (s *LoopSuite) TestCheckCharmUpgradeIncorrectLXDProfile(c *tc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     testhelpers.Stub{},
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

func (s *LoopSuite) testCheckCharmUpgradeDoesNothing(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	s.resolver = resolver.ResolverFunc(func(
		_ context.Context,
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		return nil, resolver.ErrWaiting
	})

	cancel()

	_, err := s.loop(c)(ctx)
	c.Assert(err, tc.Equals, resolver.ErrLoopAborted)

	// Run not called
	c.Assert(s.executor.Calls(), tc.HasLen, 3)
	s.executor.CheckCallNames(c, "State", "State", "State")
}

func (s *LoopSuite) TestCheckCharmUpgrade(c *tc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     testhelpers.Stub{},
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

func (s *LoopSuite) TestCheckCharmUpgradeMissingCharmDir(c *tc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     testhelpers.Stub{},
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

func (s *LoopSuite) TestCheckCharmInstallMissingCharmDirInstallHookFail(c *tc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     testhelpers.Stub{},
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

func (s *LoopSuite) TestCheckCharmUpgradeLXDProfile(c *tc.C) {
	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     testhelpers.Stub{},
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

func (s *LoopSuite) testCheckCharmUpgradeCallsRun(c *tc.C, op string) {
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	s.opFactory = &mockOpFactory{
		Factory: nil,
		Stub:    testhelpers.Stub{},
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

	cancel()

	_, err := s.loop(c)(ctx)
	c.Assert(err, tc.Equals, resolver.ErrLoopAborted)

	// Run not called
	c.Assert(s.executor.Calls(), tc.HasLen, 4)
	s.executor.CheckCallNames(c, "State", "State", "Run", "State")

	c.Assert(s.opFactory.Calls(), tc.HasLen, 1)
	s.opFactory.CheckCallNames(c, "New"+op)
}

func (s *LoopSuite) TestCancelledLockAcquisitionCausesRestart(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	s.executor = &mockOpExecutor{
		Executor: nil,
		Stub:     testhelpers.Stub{},
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

	_, err := s.loop(c)(ctx)
	c.Assert(err, tc.Equals, resolver.ErrRestart)
}

func waitChannel(c *tc.C, ch <-chan interface{}, activity string) interface{} {
	select {
	case v := <-ch:
		return v
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out %s", activity)
		panic("unreachable")
	}
}
