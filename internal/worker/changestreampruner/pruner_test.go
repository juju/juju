// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package changestreampruner

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/changestream"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type prunerWorkerSuite struct {
	baseSuite
}

func TestPrunerWorkerSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &prunerWorkerSuite{})
}

func (s *prunerWorkerSuite) TestPrunerDies(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// No update to the window, as there is nothing to prune.
	done := make(chan struct{})

	s.expectClock()
	s.expectTimerRepeated(1, done)

	s.changeStreamService.EXPECT().Prune(gomock.Any(), changestream.Window{}).Return(changestream.Window{}, int64(0), nil)

	pruner := s.newPruner(c)
	defer workertest.CleanKill(c, pruner)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for pruner to run")
	}
}

func (s *prunerWorkerSuite) TestPrunerUpdatesWindow(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// No update to the window, as there is nothing to prune.
	done := make(chan struct{})

	s.expectClock()
	s.expectTimerRepeated(2, done)

	now := time.Now().Truncate(time.Second)
	s.changeStreamService.EXPECT().Prune(gomock.Any(), changestream.Window{}).Return(changestream.Window{
		Start: now,
		End:   now.Add(time.Hour),
	}, int64(0), nil)
	s.changeStreamService.EXPECT().Prune(gomock.Any(), changestream.Window{
		Start: now,
		End:   now.Add(time.Hour),
	}).Return(changestream.Window{}, int64(0), nil)

	pruner := s.newPruner(c)
	defer workertest.CleanKill(c, pruner)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for pruner to run")
	}
}

func (s *prunerWorkerSuite) newPruner(c *tc.C) *Pruner {
	w, err := NewWorker(WorkerConfig{
		ChangeStreamService: s.changeStreamService,
		Clock:               s.clock,
		Logger:              loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	return w.(*Pruner)
}
