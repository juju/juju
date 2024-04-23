// Copyright 2033 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"
	"database/sql"
	"errors"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/eventsource"
	schematesting "github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/testing"
)

type watcherSuite struct {
	schematesting.ControllerSuite

	sub    *MockSubscription
	events *MockEventSource
}

var _ = gc.Suite(&watcherSuite{})

func (*watcherSuite) TestNewUUIDsWatcherFail(c *gc.C) {
	factory := NewWatcherFactory(func() (changestream.WatchableDB, error) {
		return nil, errors.New("fail getting db instance")
	}, nil)

	_, err := factory.NewUUIDsWatcher("random_namespace", changestream.All)
	c.Assert(err, gc.ErrorMatches, "creating base watcher: fail getting db instance")
}

func (s *watcherSuite) TestNewUUIDsWatcherSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSourceWithSub()

	factory := NewWatcherFactory(func() (changestream.WatchableDB, error) {
		return &watchableDB{
			TxnRunner:   s.TxnRunner(),
			EventSource: s.events,
		}, nil
	}, nil)

	w, err := factory.NewUUIDsWatcher("external_controller", changestream.All)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-w.Changes():
	case <-time.After(jujutesting.ShortWait):
		c.Fatal("timed out waiting for change event")
	}

	workertest.CleanKill(c, w)
}

func (s *watcherSuite) TestNewNamespaceWatcherSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSourceWithSub()

	s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			CREATE TABLE some_namespace (
				uuid TEXT PRIMARY KEY
			);
		`)
		return err
	})

	factory := NewWatcherFactory(func() (changestream.WatchableDB, error) {
		return &watchableDB{
			TxnRunner:   s.TxnRunner(),
			EventSource: s.events,
		}, nil
	}, nil)

	w, err := factory.NewNamespaceWatcher(
		"some-namespace", changestream.All,
		eventsource.InitialNamespaceChanges("SELECT uuid from some_namespace"),
	)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-w.Changes():
	case <-time.After(jujutesting.ShortWait):
		c.Fatal("timed out waiting for change event")
	}

	workertest.CleanKill(c, w)
}

func (s *watcherSuite) TestNewNamespacePredicateWatcherSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSourceWithSub()

	s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			CREATE TABLE some_namespace (
				uuid TEXT PRIMARY KEY
			);
		`)
		return err
	})

	factory := NewWatcherFactory(func() (changestream.WatchableDB, error) {
		return &watchableDB{
			TxnRunner:   s.TxnRunner(),
			EventSource: s.events,
		}, nil
	}, nil)

	w, err := factory.NewNamespacePredicateWatcher(
		"some-namespace", changestream.All,
		eventsource.InitialNamespaceChanges("SELECT uuid from some_namespace"),
		func(ctx context.Context, tr database.TxnRunner, ce []changestream.ChangeEvent) (bool, error) {
			return true, nil
		})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-w.Changes():
	case <-time.After(jujutesting.ShortWait):
		c.Fatal("timed out waiting for change event")
	}

	workertest.CleanKill(c, w)
}

func (s *watcherSuite) TestNewValueWatcherSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSourceWithSub()

	factory := NewWatcherFactory(func() (changestream.WatchableDB, error) {
		return &watchableDB{
			TxnRunner:   s.TxnRunner(),
			EventSource: s.events,
		}, nil
	}, nil)

	w, err := factory.NewValueWatcher("some-namespace", "some-id-from-namespace", changestream.All)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-w.Changes():
	case <-time.After(jujutesting.ShortWait):
		c.Fatal("timed out waiting for change event")
	}

	workertest.CleanKill(c, w)
}

func (s *watcherSuite) TestNewValuePredicateWatcherSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSourceWithSub()

	factory := NewWatcherFactory(func() (changestream.WatchableDB, error) {
		return &watchableDB{
			TxnRunner:   s.TxnRunner(),
			EventSource: s.events,
		}, nil
	}, nil)

	w, err := factory.NewValuePredicateWatcher("some-namespace", "some-id-from-namespace", changestream.All, func(ctx context.Context, tr database.TxnRunner, ce []changestream.ChangeEvent) (bool, error) {
		return true, nil
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-w.Changes():
	case <-time.After(jujutesting.ShortWait):
		c.Fatal("timed out waiting for change event")
	}

	workertest.CleanKill(c, w)
}

func (s *watcherSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.sub = NewMockSubscription(ctrl)
	s.events = NewMockEventSource(ctrl)

	return ctrl
}

func (s *watcherSuite) expectSourceWithSub() {
	changes := make(chan []changestream.ChangeEvent)
	done := make(chan struct{})

	// These expectations are very soft.
	// We are only testing that the factory produces a functioning worker.
	// The workers themselves are properly tested at their package sites.
	s.sub.EXPECT().Changes().Return(changes)
	s.sub.EXPECT().Unsubscribe()
	s.sub.EXPECT().Done().Return(done).AnyTimes()

	s.events.EXPECT().Subscribe(gomock.Any()).Return(s.sub, nil)
}

type watchableDB struct {
	database.TxnRunner
	changestream.EventSource
}
