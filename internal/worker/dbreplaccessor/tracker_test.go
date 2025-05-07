// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbreplaccessor

import (
	"context"
	sql "database/sql"
	"errors"
	time "time"

	sqlair "github.com/canonical/sqlair"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testing"
)

type trackedDBReplWorkerSuite struct {
	dbBaseSuite

	states chan string
}

var _ = tc.Suite(&trackedDBReplWorkerSuite{})

func (s *trackedDBReplWorkerSuite) TestWorkerStartup(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := NewTrackedDBWorker(context.Background(), s.dbApp, "controller", WithClock(s.clock), WithLogger(s.logger))
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *trackedDBReplWorkerSuite) TestWorkerDBIsNotNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := s.newTrackedDBWorker()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	err = w.StdTxn(context.Background(), func(_ context.Context, tx *sql.Tx) error {
		if tx == nil {
			return errors.New("nil transaction")
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *trackedDBReplWorkerSuite) TestWorkerStdTxnIsNotNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := s.newTrackedDBWorker()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	done := make(chan struct{})
	err = w.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		defer close(done)

		if tx == nil {
			return errors.New("nil transaction")
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

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

	w, err := s.newTrackedDBWorker()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	done := make(chan struct{})
	err = w.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		defer close(done)

		if tx == nil {
			return errors.New("nil transaction")
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	workertest.CleanKill(c, w)
}

func (s *trackedDBReplWorkerSuite) newTrackedDBWorker() (TrackedDB, error) {
	return newTrackedDBWorker(context.Background(),
		s.states,
		s.dbApp, "controller",
		WithClock(s.clock),
		WithLogger(s.logger),
	)
}
