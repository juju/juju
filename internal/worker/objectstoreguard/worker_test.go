// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreguard

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/fortress"
)

func TestWorkerSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &workerSuite{})
	})
}

type workerSuite struct{}

func (s *workerSuite) TestUnusableStoreRemainsLocked(c *tc.C) {
	service, changes := newDrainingService()
	guard := &recordingGuard{calls: make(chan string, 1)}
	flusher := &recordingFlusher{calls: make(chan struct{}, 1)}
	w, err := NewWorker(Config{
		Guard:              guard,
		DrainingService:    service,
		ObjectStoreFlusher: flusher,
		Logger:             loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	send(c, service.phases, objectstore.PhaseUnknown)
	send(c, service.usable, false)
	send(c, changes, struct{}{})
	assertGuardCall(c, guard.calls, "lockdown")
	select {
	case <-flusher.calls:
		c.Fatal("unexpected object-store flush")
	default:
	}
}

func (s *workerSuite) TestRefreshFailurePreventsUnlock(c *tc.C) {
	service, changes := newDrainingService()
	guard := &recordingGuard{calls: make(chan string, 1)}
	flusher := &recordingFlusher{
		calls: make(chan struct{}, 1),
		err:   errors.New("boom"),
	}
	w, err := NewWorker(Config{
		Guard:              guard,
		DrainingService:    service,
		ObjectStoreFlusher: flusher,
		Logger:             loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	send(c, service.phases, objectstore.PhaseCompleted)
	send(c, service.usable, true)
	send(c, changes, struct{}{})
	assertFlushCall(c, flusher.calls)
	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, `refreshing local object stores: boom`)
	select {
	case call := <-guard.calls:
		c.Fatalf("unexpected guard call %q", call)
	default:
	}
}

func (s *workerSuite) TestUsabilityCheckedOnceBeforeRefreshAndUnlock(c *tc.C) {
	service, changes := newDrainingService()
	events := make(chan string, 4)
	service.events = events
	guard := &orderedGuard{events: events}
	flusher := &orderedFlusher{events: events}
	w, err := NewWorker(Config{
		Guard:              guard,
		DrainingService:    service,
		ObjectStoreFlusher: flusher,
		Logger:             loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	send(c, service.phases, objectstore.PhaseCompleted)
	send(c, service.usable, true)
	send(c, changes, struct{}{})

	for _, expected := range []string{"check", "flush", "unlock"} {
		select {
		case event := <-events:
			c.Check(event, tc.Equals, expected)
		case <-c.Context().Done():
			c.Fatalf("expected event %q", expected)
		}
	}
	select {
	case event := <-events:
		c.Fatalf("unexpected event %q", event)
	default:
	}
}

func (s *workerSuite) TestGuardTracksDrainingLifecycle(c *tc.C) {
	service, changes := newDrainingService()
	guard := &recordingGuard{calls: make(chan string, 3)}
	flusher := &recordingFlusher{calls: make(chan struct{}, 2)}
	w, err := NewWorker(Config{
		Guard:              guard,
		DrainingService:    service,
		ObjectStoreFlusher: flusher,
		Logger:             loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	send(c, service.phases, objectstore.PhaseUnknown)
	send(c, service.usable, true)
	send(c, changes, struct{}{})
	assertFlushCall(c, flusher.calls)
	assertGuardCall(c, guard.calls, "unlock")

	send(c, service.phases, objectstore.PhaseDraining)
	send(c, changes, struct{}{})
	assertGuardCall(c, guard.calls, "lockdown")

	send(c, service.phases, objectstore.PhaseCompleted)
	send(c, service.usable, true)
	send(c, changes, struct{}{})
	assertFlushCall(c, flusher.calls)
	assertGuardCall(c, guard.calls, "unlock")
}

func (s *workerSuite) TestErrorPhaseKeepsGuardLocked(c *tc.C) {
	service, changes := newDrainingService()
	guard := &recordingGuard{calls: make(chan string, 1)}
	w, err := NewWorker(Config{
		Guard:              guard,
		DrainingService:    service,
		ObjectStoreFlusher: &recordingFlusher{calls: make(chan struct{}, 1)},
		Logger:             loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	send(c, service.phases, objectstore.PhaseError)
	send(c, changes, struct{}{})
	assertGuardCall(c, guard.calls, "lockdown")
}

func newDrainingService() (*drainingService, chan struct{}) {
	changes := make(chan struct{}, 3)
	return &drainingService{
		watcher: watchertest.NewMockNotifyWatcher(changes),
		phases:  make(chan objectstore.Phase, 3),
		usable:  make(chan bool, 4),
	}, changes
}

func assertGuardCall(c *tc.C, calls <-chan string, expected string) {
	select {
	case call := <-calls:
		c.Check(call, tc.Equals, expected)
	case <-c.Context().Done():
		c.Fatalf("expected guard call %q", expected)
	}
}

func assertFlushCall(c *tc.C, calls <-chan struct{}) {
	select {
	case <-calls:
	case <-c.Context().Done():
		c.Fatalf("expected object-store flush")
	}
}

func send[T any](c *tc.C, ch chan<- T, value T) {
	select {
	case ch <- value:
	case <-c.Context().Done():
		c.Fatalf("timed out sending test value")
	}
}

type drainingService struct {
	watcher watcher.NotifyWatcher
	phases  chan objectstore.Phase
	usable  chan bool
	events  chan<- string
}

func (s *drainingService) IsObjectStoreUsable(ctx context.Context) (bool, error) {
	if s.events != nil {
		select {
		case s.events <- "check":
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	select {
	case usable := <-s.usable:
		return usable, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (s *drainingService) GetDrainingPhase(ctx context.Context) (objectstore.Phase, error) {
	select {
	case phase := <-s.phases:
		return phase, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *drainingService) WatchDraining(context.Context) (watcher.NotifyWatcher, error) {
	return s.watcher, nil
}

type recordingGuard struct {
	calls chan string
}

func (g *recordingGuard) Unlock(ctx context.Context) error {
	select {
	case g.calls <- "unlock":
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *recordingGuard) Lockdown(ctx context.Context) error {
	select {
	case g.calls <- "lockdown":
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

var _ fortress.Guard = (*recordingGuard)(nil)

type recordingFlusher struct {
	calls chan struct{}
	err   error
}

func (f *recordingFlusher) FlushWorkers(ctx context.Context) error {
	select {
	case f.calls <- struct{}{}:
		return f.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

var _ objectstore.ObjectStoreFlusher = (*recordingFlusher)(nil)

type orderedGuard struct {
	events chan<- string
}

func (g *orderedGuard) Unlock(ctx context.Context) error {
	select {
	case g.events <- "unlock":
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *orderedGuard) Lockdown(ctx context.Context) error {
	select {
	case g.events <- "lockdown":
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type orderedFlusher struct {
	events chan<- string
}

func (f *orderedFlusher) FlushWorkers(ctx context.Context) error {
	select {
	case f.events <- "flush":
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
