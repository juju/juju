// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbreplaccessor

import (
	"context"
	sql "database/sql"
	"errors"
	stdtesting "testing"
	time "time"

	sqlair "github.com/canonical/sqlair"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testing"
)

type trackedDBReplWorkerSuite struct {
	dbBaseSuite

	states chan string
}

func TestTrackedDBReplWorkerSuite(t *stdtesting.T) {
	tc.Run(t, &trackedDBReplWorkerSuite{})
}
func (s *trackedDBReplWorkerSuite) TestWorkerStartup(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := NewTrackedDBWorker(c.Context(), s.dbApp, "controller", WithClock(s.clock), WithLogger(s.logger))
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *trackedDBReplWorkerSuite) TestWorkerDBIsNotNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := s.newTrackedDBWorker(c)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	err = w.StdTxn(c.Context(), func(_ context.Context, tx *sql.Tx) error {
		if tx == nil {
			return errors.New("nil transaction")
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *trackedDBReplWorkerSuite) TestWorkerStdTxnIsNotNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := s.newTrackedDBWorker(c)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	done := make(chan struct{})
	err = w.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		defer close(done)

		if tx == nil {
			return errors.New("nil transaction")
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	workertest.CleanKill(c, w)
}

func (s *trackedDBReplWorkerSuite) TestWorkerTxnIsNotNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := s.newTrackedDBWorker(c)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	done := make(chan struct{})
	err = w.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		defer close(done)

		if tx == nil {
			return errors.New("nil transaction")
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	workertest.CleanKill(c, w)
}

func (s *trackedDBReplWorkerSuite) newTrackedDBWorker(c *tc.C) (TrackedDB, error) {
	return newTrackedDBWorker(c.Context(),
		s.states,
		s.dbApp, "controller",
		WithClock(s.clock),
		WithLogger(s.logger),
	)
}
