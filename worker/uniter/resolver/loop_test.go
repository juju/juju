// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver_test

import (
	"errors"
	"time"

	"github.com/juju/charm/v7"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

type LoopSuite struct {
	testing.BaseSuite

	resolver  resolver.Resolver
	watcher   *mockRemoteStateWatcher
	opFactory *mockOpFactory
	executor  *mockOpExecutor
	charmURL  *charm.URL
	abort     chan struct{}
	onIdle    func() error
}

var _ = gc.Suite(&LoopSuite{})

func (s *LoopSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resolver = resolver.ResolverFunc(func(resolver.LocalState, remotestate.Snapshot, operation.Factory) (operation.Operation, error) {
		return nil, resolver.ErrNoOperation
	})
	s.watcher = &mockRemoteStateWatcher{
		changes: make(chan struct{}, 1),
	}
	s.opFactory = &mockOpFactory{}
	s.executor = &mockOpExecutor{}
	s.charmURL = charm.MustParseURL("cs:trusty/mysql")
	s.abort = make(chan struct{})
}

func (s *LoopSuite) loop() (resolver.LocalState, error) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
	}
	err := resolver.Loop(resolver.LoopConfig{
		Resolver:      s.resolver,
		Factory:       s.opFactory,
		Watcher:       s.watcher,
		Executor:      s.executor,
		Abort:         s.abort,
		OnIdle:        s.onIdle,
		CharmDirGuard: &mockCharmDirGuard{},
	}, &localState)
	return localState, err
}

func (s *LoopSuite) TestAbort(c *gc.C) {
	close(s.abort)
	_, err := s.loop()
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
		_, err := s.loop()
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
	_, err := s.loop()
	c.Assert(err, gc.ErrorMatches, "onIdle failed")
}

func (s *LoopSuite) TestErrWaitingNoOnIdle(c *gc.C) {
	var onIdleCalled bool
	s.onIdle = func() error {
		onIdleCalled = true
		return nil
	}
	s.resolver = resolver.ResolverFunc(func(
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		return nil, resolver.ErrWaiting
	})
	close(s.abort)
	_, err := s.loop()
	c.Assert(err, gc.Equals, resolver.ErrLoopAborted)
	c.Assert(onIdleCalled, jc.IsFalse)
}

func (s *LoopSuite) TestInitialFinalLocalState(c *gc.C) {
	var local resolver.LocalState
	s.resolver = resolver.ResolverFunc(func(
		l resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		local = l
		return nil, resolver.ErrNoOperation
	})

	close(s.abort)
	lastLocal, err := s.loop()
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

	_, err := s.loop()
	c.Assert(err, gc.Equals, resolver.ErrLoopAborted)
	c.Assert(resolverCalls, gc.Equals, 3)
	s.executor.CheckCallNames(c, "State", "State", "Run", "State", "State")

	runArgs := s.executor.Calls()[2].Args
	c.Assert(runArgs, gc.HasLen, 2)
	c.Assert(runArgs[0], gc.DeepEquals, theOp)
	c.Assert(runArgs[1], gc.NotNil)
}

func (s *LoopSuite) TestLoopWithChange(c *gc.C) {
	var resolverCalls int
	theOp := &mockOp{}
	s.resolver = resolver.ResolverFunc(func(
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

	_, err := s.loop()
	c.Assert(err, gc.Equals, resolver.ErrLoopAborted)
	c.Assert(resolverCalls, gc.Equals, 4)
	s.executor.CheckCallNames(c, "State", "State", "Run", "State", "State", "State")

	c.Assert(remoteStateSnapshotCount, gc.Equals, 5)
	select {
	case _, ok := <-remoteStateSnapshotChan:
		c.Assert(ok, jc.IsTrue)
		c.Fatalf("remote state snapshot channel fired more than once")
	default:
	}

	runArgs := s.executor.Calls()[2].Args
	c.Assert(runArgs, gc.HasLen, 2)
	c.Assert(runArgs[0], gc.DeepEquals, theOp)
	c.Assert(runArgs[1], gc.NotNil)
}

func (s *LoopSuite) TestRunFails(c *gc.C) {
	s.executor.SetErrors(errors.New("Run fails"))
	s.resolver = resolver.ResolverFunc(func(
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		return mockOp{}, nil
	})
	_, err := s.loop()
	c.Assert(err, gc.ErrorMatches, "Run fails")
}

func (s *LoopSuite) TestNextOpFails(c *gc.C) {
	s.resolver = resolver.ResolverFunc(func(
		_ resolver.LocalState,
		_ remotestate.Snapshot,
		_ operation.Factory,
	) (operation.Operation, error) {
		return nil, errors.New("NextOp fails")
	})
	_, err := s.loop()
	c.Assert(err, gc.ErrorMatches, "NextOp fails")
}

func waitChannel(c *gc.C, ch <-chan interface{}, activity string) interface{} {
	select {
	case v := <-ch:
		return v
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out " + activity)
		panic("unreachable")
	}
}
