// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreguard

import (
	"context"
	"testing"

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

func (s *workerSuite) TestGuardTracksDrainingLifecycle(c *tc.C) {
	service, changes := newDrainingService()
	guard := &recordingGuard{calls: make(chan string, 3)}
	w, err := NewWorker(Config{
		Guard:           guard,
		DrainingService: service,
		Logger:          loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	service.phases <- objectstore.PhaseUnknown
	changes <- struct{}{}
	assertGuardCall(c, guard.calls, "unlock")

	service.phases <- objectstore.PhaseDraining
	changes <- struct{}{}
	assertGuardCall(c, guard.calls, "lockdown")

	service.phases <- objectstore.PhaseCompleted
	changes <- struct{}{}
	assertGuardCall(c, guard.calls, "unlock")
}

func (s *workerSuite) TestErrorPhaseKeepsGuardLocked(c *tc.C) {
	service, changes := newDrainingService()
	guard := &recordingGuard{calls: make(chan string, 1)}
	w, err := NewWorker(Config{
		Guard:           guard,
		DrainingService: service,
		Logger:          loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	service.phases <- objectstore.PhaseError
	changes <- struct{}{}
	assertGuardCall(c, guard.calls, "lockdown")
}

func newDrainingService() (*drainingService, chan struct{}) {
	changes := make(chan struct{}, 3)
	return &drainingService{
		watcher: watchertest.NewMockNotifyWatcher(changes),
		phases:  make(chan objectstore.Phase, 3),
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

type drainingService struct {
	watcher watcher.NotifyWatcher
	phases  chan objectstore.Phase
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

func (g *recordingGuard) Unlock(context.Context) error {
	g.calls <- "unlock"
	return nil
}

func (g *recordingGuard) Lockdown(context.Context) error {
	g.calls <- "lockdown"
	return nil
}

var _ fortress.Guard = (*recordingGuard)(nil)
