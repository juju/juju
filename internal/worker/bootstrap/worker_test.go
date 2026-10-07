// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock/testclock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/flags"
	coremodel "github.com/juju/juju/core/model"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/statushistory"
	"github.com/juju/juju/internal/testhelpers"
)

type workerSuite struct {
	baseSuite
}

func TestWorkerSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &workerSuite{})
	})
}

func (s *workerSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	for _, test := range []struct {
		name       string
		invalidate func(*WorkerConfig)
	}{
		{"Operation", func(cfg *WorkerConfig) { cfg.Operation = nil }},
		{"FlagService", func(cfg *WorkerConfig) { cfg.FlagService = nil }},
		{"BootstrapUnlocker", func(cfg *WorkerConfig) { cfg.BootstrapUnlocker = nil }},
		{"StatusHistory", func(cfg *WorkerConfig) { cfg.StatusHistory = nil }},
		{"Logger", func(cfg *WorkerConfig) { cfg.Logger = nil }},
		{"Clock", func(cfg *WorkerConfig) { cfg.Clock = nil }},
	} {
		cfg := s.config(c)
		c.Assert(cfg.Validate(), tc.ErrorIsNil)
		test.invalidate(&cfg)
		_, err := NewWorker(cfg)
		c.Check(err, tc.ErrorIs, jujuerrors.NotValid, tc.Commentf("%s", test.name))
	}

	cfg := s.config(c)
	cfg.ControllerModelUUID = "invalid"
	_, err := NewWorker(cfg)
	c.Check(err, tc.ErrorMatches, "controller model id: .*")
}

func (s *workerSuite) TestCompletionOrder(c *tc.C) {
	defer s.setupMocks(c).Finish()
	cfg := s.config(c)
	var events []string
	cfg.Operation = func(context.Context) (func(), error) {
		events = append(events, "operation")
		return func() { events = append(events, "cleanup") }, nil
	}
	s.flagService.EXPECT().SetFlag(gomock.Any(), flags.BootstrapFlag, true, flags.BootstrapFlagDescription).
		DoAndReturn(func(context.Context, string, bool, string) error {
			events = append(events, "flag")
			return nil
		})
	s.bootstrapUnlocker.EXPECT().Unlock().Do(func() { events = append(events, "unlock") })
	cfg.StatusHistory = statusHistoryFunc(func(_ context.Context, ns statushistory.Namespace, info corestatus.StatusInfo) error {
		events = append(events, "history")
		c.Check(ns, tc.Equals, status.ModelNamespace.WithID(cfg.ControllerModelUUID.String()))
		c.Check(info.Status, tc.Equals, corestatus.Available)
		c.Assert(info.Since, tc.NotNil)
		c.Check(*info.Since, tc.Equals, cfg.Clock.Now())
		return nil
	})
	states := make(chan string, 2)
	w, err := newWorker(cfg, states)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	c.Assert(workertest.CheckKilled(c, w), tc.ErrorIsNil)
	c.Check(events, tc.DeepEquals, []string{"operation", "flag", "cleanup", "unlock", "history"})
	c.Assert(len(states), tc.Equals, 2)
	c.Check(<-states, tc.Equals, stateStarted)
	c.Check(<-states, tc.Equals, stateCompleted)
}

func (s *workerSuite) TestOperationFailureDoesNotComplete(c *tc.C) {
	defer s.setupMocks(c).Finish()
	cfg := s.config(c)
	expected := errors.New("operation failed")
	cleaned := false
	cfg.Operation = func(context.Context) (func(), error) {
		return func() { cleaned = true }, expected
	}
	states := make(chan string, 2)
	w, err := newWorker(cfg, states)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	c.Check(workertest.CheckKilled(c, w), tc.ErrorIs, expected)
	c.Check(cleaned, tc.IsFalse)
	c.Assert(len(states), tc.Equals, 1)
	c.Check(<-states, tc.Equals, stateStarted)
}

func (s *workerSuite) TestFlagFailurePreservesArtefactsAndGate(c *tc.C) {
	defer s.setupMocks(c).Finish()
	cfg := s.config(c)
	expected := errors.New("flag write failed")
	cleaned := false
	cfg.Operation = func(context.Context) (func(), error) {
		return func() { cleaned = true }, nil
	}
	s.flagService.EXPECT().SetFlag(gomock.Any(), flags.BootstrapFlag, true, flags.BootstrapFlagDescription).Return(expected)
	states := make(chan string, 2)
	w, err := newWorker(cfg, states)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	c.Check(workertest.CheckKilled(c, w), tc.ErrorIs, expected)
	c.Check(cleaned, tc.IsFalse)
	c.Assert(len(states), tc.Equals, 1)
	c.Check(<-states, tc.Equals, stateStarted)
}

func (s *workerSuite) TestKillCancelsOperation(c *tc.C) {
	defer s.setupMocks(c).Finish()
	cfg := s.config(c)
	started := make(chan struct{})
	cfg.Operation = func(ctx context.Context) (func(), error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	select {
	case <-started:
	case <-c.Context().Done():
		c.Fatal("operation did not start")
	}
	w.Kill()
	c.Check(workertest.CheckKilled(c, w), tc.ErrorIs, context.Canceled)
}

func (s *workerSuite) TestStatusHistoryFailureDoesNotFailCompletion(c *tc.C) {
	defer s.setupMocks(c).Finish()
	cfg := s.config(c)
	s.flagService.EXPECT().SetFlag(gomock.Any(), flags.BootstrapFlag, true, flags.BootstrapFlagDescription).Return(nil)
	s.bootstrapUnlocker.EXPECT().Unlock()
	recorded := false
	cfg.StatusHistory = statusHistoryFunc(func(context.Context, statushistory.Namespace, corestatus.StatusInfo) error {
		recorded = true
		return errors.New("history unavailable")
	})
	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	c.Assert(workertest.CheckKilled(c, w), tc.ErrorIsNil)
	c.Check(recorded, tc.IsTrue)
}

func (s *workerSuite) config(c *tc.C) WorkerConfig {
	return WorkerConfig{
		Operation:           func(context.Context) (func(), error) { return nil, nil },
		FlagService:         s.flagService,
		BootstrapUnlocker:   s.bootstrapUnlocker,
		ControllerModelUUID: tc.Must0(c, coremodel.NewUUID),
		StatusHistory: statusHistoryFunc(func(context.Context, statushistory.Namespace, corestatus.StatusInfo) error {
			c.Error("unexpected status history write")
			return nil
		}),
		Logger: s.logger,
		Clock:  testclock.NewClock(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)),
	}
}

type statusHistoryFunc func(context.Context, statushistory.Namespace, corestatus.StatusInfo) error

func (f statusHistoryFunc) RecordStatus(ctx context.Context, ns statushistory.Namespace, info corestatus.StatusInfo) error {
	return f(ctx, ns, info)
}
