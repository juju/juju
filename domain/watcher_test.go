// Copyright 2033 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/eventsource"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	jujutesting "github.com/juju/juju/internal/testing"
)

type watcherSuite struct {
	schematesting.ControllerSuite

	sub    *MockSubscription
	events *MockEventSource
}

func TestWatcherSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &watcherSuite{})
}

func (*watcherSuite) TestNewUUIDsWatcherFail(c *tc.C) {
	factory := NewWatcherFactory(func(context.Context) (changestream.WatchableDB, error) {
		return nil, errors.New("fail getting db instance")
	}, nil)

	_, err := factory.NewUUIDsWatcher(c.Context(), "random_namespace", changestream.All)
	c.Assert(err, tc.ErrorMatches, "creating base watcher: fail getting db instance")
}

func (s *watcherSuite) TestNewUUIDsWatcherSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSourceWithSub()

	factory := NewWatcherFactory(func(context.Context) (changestream.WatchableDB, error) {
		return &watchableDB{
			TxnRunner:   s.TxnRunner(),
			EventSource: s.events,
		}, nil
	}, nil)

	w, err := factory.NewUUIDsWatcher(c.Context(), "external_controller", changestream.All)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-w.Changes():
	case <-time.After(jujutesting.ShortWait):
		c.Fatal("timed out waiting for change event")
	}

	workertest.CleanKill(c, w)
}

func (s *watcherSuite) TestNewNamespaceWatcherSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSourceWithSub()

	s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			CREATE TABLE some_namespace (
				uuid TEXT NOT NULL PRIMARY KEY
			);
		`)
		return err
	})

	factory := NewWatcherFactory(func(context.Context) (changestream.WatchableDB, error) {
		return &watchableDB{
			TxnRunner:   s.TxnRunner(),
			EventSource: s.events,
		}, nil
	}, nil)

	w, err := factory.NewNamespaceWatcher(
		c.Context(),
		eventsource.InitialNamespaceChanges("SELECT uuid from some_namespace"),
		eventsource.NamespaceFilter("some_namespace", changestream.All),
	)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-w.Changes():
	case <-time.After(jujutesting.ShortWait):
		c.Fatal("timed out waiting for change event")
	}

	workertest.CleanKill(c, w)
}

func (s *watcherSuite) TestNewNamespaceMapperWatcherSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSourceWithSub()

	s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			CREATE TABLE some_namespace (
				uuid TEXT NOT NULL PRIMARY KEY
			);
		`)
		return err
	})

	factory := NewWatcherFactory(func(context.Context) (changestream.WatchableDB, error) {
		return &watchableDB{
			TxnRunner:   s.TxnRunner(),
			EventSource: s.events,
		}, nil
	}, nil)

	w, err := factory.NewNamespaceMapperWatcher(
		c.Context(),
		eventsource.InitialNamespaceChanges("SELECT uuid from some_namespace"),
		func(ctx context.Context, ce []changestream.ChangeEvent) ([]string, error) {
			return transform.Slice(ce, func(c changestream.ChangeEvent) string {
				return c.Changed()
			}), nil
		},
		eventsource.NamespaceFilter("some_namespace", changestream.All),
	)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-w.Changes():
	case <-time.After(jujutesting.ShortWait):
		c.Fatal("timed out waiting for change event")
	}

	workertest.CleanKill(c, w)
}

func (s *watcherSuite) setupMocks(c *tc.C) *gomock.Controller {
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
	s.sub.EXPECT().Kill()
	s.sub.EXPECT().Done().Return(done).AnyTimes()

	s.events.EXPECT().Subscribe(gomock.Any()).Return(s.sub, nil)
}

type watchableDB struct {
	database.TxnRunner
	changestream.EventSource
}
