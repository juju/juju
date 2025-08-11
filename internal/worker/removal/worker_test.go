// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

import (
	"context"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/removal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type workerSuite struct {
	testhelpers.IsolationSuite

	svc *MockRemovalService
	clk *MockClock
}

func TestWorkerSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestWorkerStartStop(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	ch := make(chan []string)
	watch := watchertest.NewMockStringsWatcher(ch)
	s.svc.EXPECT().WatchRemovals(gomock.Any()).Return(watch, nil)

	// Use the timer creation as a synchronisation point below.
	// so that we know we are entrant into the worker's loop.
	sync := make(chan struct{})
	s.clk.EXPECT().NewTimer(jobCheckMaxInterval).DoAndReturn(func(d time.Duration) clock.Timer {
		sync <- struct{}{}
		return clock.WallClock.NewTimer(d)
	})

	cfg := Config{
		RemovalService: s.svc,
		Clock:          s.clk,
		Logger:         loggertesting.WrapCheckLog(c),
	}
	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-sync:
	case <-time.After(testhelpers.ShortWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, w)
}

// TestWorkerNotifiedSchedulesDueJob tests the following sequence of events:
// - The watcher fires.
// - We query for jobs, receive two, but only one is due for execution,
// - Only the due job is scheduled with the runner.
func (s *workerSuite) TestWorkerNotifiedSchedulesDueJob(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	ch := make(chan []string)
	watch := watchertest.NewMockStringsWatcher(ch)
	s.svc.EXPECT().WatchRemovals(gomock.Any()).Return(watch, nil)

	s.clk.EXPECT().NewTimer(jobCheckMaxInterval).DoAndReturn(func(d time.Duration) clock.Timer {
		return clock.WallClock.NewTimer(d)
	})

	now := time.Now().UTC()
	s.clk.EXPECT().Now().Return(now).Times(2)

	dueJob := removal.Job{
		UUID:         "due-job-uuid",
		RemovalType:  0,
		EntityUUID:   "due-relation-uuid",
		Force:        false,
		ScheduledFor: now.Add(-time.Hour),
	}
	laterJob := removal.Job{
		UUID:         "later-job-uuid",
		RemovalType:  0,
		EntityUUID:   "later-relation-uuid",
		Force:        false,
		ScheduledFor: now.Add(time.Hour),
	}
	s.svc.EXPECT().GetAllJobs(gomock.Any()).Return([]removal.Job{dueJob, laterJob}, nil)

	// Use job execution as a synchronisation point below.
	// so that we know we can kill the worker.
	sync := make(chan struct{})
	s.svc.EXPECT().ExecuteJob(gomock.Any(), dueJob).DoAndReturn(func(_ context.Context, job removal.Job) error {
		sync <- struct{}{}
		return nil
	})

	cfg := Config{
		RemovalService: s.svc,
		Clock:          s.clk,
		Logger:         loggertesting.WrapCheckLog(c),
	}
	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{"some-job-uuid"}:
	case <-time.After(testhelpers.ShortWait):
		c.Fatalf("timed out waiting for watcher event consumption")
	}

	select {
	case <-sync:
	case <-time.After(testhelpers.ShortWait):
		c.Fatalf("timed out waiting for job execution")
	}

	workertest.CleanKill(c, w)
}

// TestWorkerTimerSchedulesOnlyRequiredJob tests the following sequence of events:
// - The timer fires.
// - We query for jobs, receive two, but one has already been scheduled.
// - Only the unscheduled job is scheduled with the runner.
func (s *workerSuite) TestWorkerTimerSchedulesOnlyRequiredJob(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	ch := make(chan []string)
	watch := watchertest.NewMockStringsWatcher(ch)
	s.svc.EXPECT().WatchRemovals(gomock.Any()).Return(watch, nil)

	timerChan := make(chan time.Time)
	timer := NewMockTimer(ctrl)
	timer.EXPECT().Chan().Return(timerChan).MaxTimes(2)
	timer.EXPECT().Reset(gomock.Any()).Return(true)
	timer.EXPECT().Stop().Return(true)
	s.clk.EXPECT().NewTimer(jobCheckMaxInterval).Return(timer)

	now := time.Now().UTC()
	s.clk.EXPECT().Now().Return(now).AnyTimes()

	dueJob := removal.Job{
		UUID:         "due-job-uuid",
		RemovalType:  0,
		EntityUUID:   "due-relation-uuid",
		Force:        false,
		ScheduledFor: now.Add(-time.Hour),
	}
	scheduledJob := removal.Job{
		UUID:         "scheduled-job-uuid",
		RemovalType:  0,
		EntityUUID:   "scheduled-relation-uuid",
		Force:        false,
		ScheduledFor: now.Add(-time.Hour),
	}
	s.svc.EXPECT().GetAllJobs(gomock.Any()).DoAndReturn(func(context.Context) ([]removal.Job, error) {
		return []removal.Job{dueJob, scheduledJob}, nil
	})

	// Use job execution as a synchronisation point below.
	// so that we know we can kill the worker.
	sync := make(chan struct{})
	s.svc.EXPECT().ExecuteJob(gomock.Any(), dueJob).DoAndReturn(func(ctx context.Context, job removal.Job) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sync <- struct{}{}:
		}
		return nil
	})

	cfg := Config{
		RemovalService: s.svc,
		Clock:          s.clk,
		Logger:         loggertesting.WrapCheckLog(c),
	}
	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Imitate a worker already scheduled in the runner.
	rw := w.(*removalWorker)
	err = rw.runner.StartWorker(c.Context(), "scheduled-job-uuid", func(ctx context.Context) (worker.Worker, error) {
		w := jobWorker{}
		w.tomb.Go(func() error {
			<-w.tomb.Dying()
			return nil
		})
		return &w, nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Once the worker is scheduled in the runner, fire the timer to ensure only
	// the required worker is started.
	select {
	case timerChan <- now:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for timer to fire")
	}

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for job execution")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerReport(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	ch := make(chan []string)
	watch := watchertest.NewMockStringsWatcher(ch)
	s.svc.EXPECT().WatchRemovals(gomock.Any()).Return(watch, nil)

	s.clk.EXPECT().NewTimer(jobCheckMaxInterval).DoAndReturn(func(d time.Duration) clock.Timer {
		return clock.WallClock.NewTimer(d)
	})

	cfg := Config{
		RemovalService: s.svc,
		Clock:          s.clk,
		Logger:         loggertesting.WrapCheckLog(c),
	}
	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Imitate two workers already scheduled in the runner.
	rw := w.(*removalWorker)

	err = rw.runner.StartWorker(c.Context(), "job-uuid-1", func(ctx context.Context) (worker.Worker, error) {
		w := jobWorker{job: removal.Job{
			UUID:        "job-uuid-1",
			RemovalType: 0,
			EntityUUID:  "relation-uuid-1",
			Force:       false,
		}}
		w.tomb.Go(func() error {
			<-w.tomb.Dying()
			return nil
		})
		return &w, nil
	})
	c.Assert(err, tc.ErrorIsNil)

	err = rw.runner.StartWorker(c.Context(), "job-uuid-2", func(ctx context.Context) (worker.Worker, error) {
		w := jobWorker{job: removal.Job{
			UUID:        "job-uuid-2",
			RemovalType: 0,
			EntityUUID:  "relation-uuid-2",
			Force:       true,
		}}
		w.tomb.Go(func() error {
			<-w.tomb.Dying()
			return nil
		})
		return &w, nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// We need to wait until the workers are actually reported as started.
	// A worker not yet running will not have the "report" key in its output
	// for the [Report] method.
	var (
		count int
		r     map[string]any
	)
	for {
		count++
		if count > 200 {
			c.Fatalf("timed out waiting for runner to schedule jobs")
		}
		time.Sleep(10 * time.Millisecond)

		if len(rw.runner.WorkerNames()) == 2 {
			r = rw.Report()
			c.Assert(r, tc.HasLen, 1)

			rm, ok := r["workers"].(map[string]any)
			c.Assert(ok, tc.IsTrue)

			j1, ok := rm["job-uuid-1"]
			c.Assert(ok, tc.IsTrue)

			j1m, ok := j1.(map[string]any)
			c.Assert(ok, tc.IsTrue)

			j1s, ok := j1m["state"].(string)
			if !(ok && j1s == "started") {
				continue
			}

			j1r, ok := j1m["report"].(map[string]any)
			c.Assert(ok, tc.IsTrue)
			c.Check(j1r["job-type"], tc.Equals, removal.RelationJob)
			c.Check(j1r["removal-entity"], tc.Equals, "relation-uuid-1")
			c.Check(j1r["force"], tc.IsFalse)

			j2, ok := rm["job-uuid-2"]
			c.Assert(ok, tc.IsTrue)

			j2m, ok := j2.(map[string]any)
			c.Assert(ok, tc.IsTrue)

			j2s, ok := j2m["state"].(string)
			if !(ok && j2s == "started") {
				continue
			}

			j2r, ok := j2m["report"].(map[string]any)
			c.Assert(ok, tc.IsTrue)
			c.Check(j2r["job-type"], tc.Equals, removal.RelationJob)
			c.Check(j2r["removal-entity"], tc.Equals, "relation-uuid-2")
			c.Check(j2r["force"], tc.IsTrue)

			break
		}
	}

	// Nicely formatted report to tell us what's going on.
	if c.Failed() {
		c.Log(spew.Sdump(r))
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.svc = NewMockRemovalService(ctrl)
	s.clk = NewMockClock(ctrl)

	return ctrl
}
