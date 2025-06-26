// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"
	"database/sql"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testing"
)

var _ watcher.StringsWatcher = &NamespaceWatcher{}

type namespaceSuite struct {
	baseSuite
}

func TestNamespaceSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &namespaceSuite{})
}

func (s *namespaceSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)
	s.ApplyDDL(c, schemaDDLApplier{})
}

func (s *namespaceSuite) TestInitialStateSent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subExp := s.sub.EXPECT()

	// We go through the worker loop twice; once to dispatch initial events,
	// then to pick up tomb.Dying(). Done() is read each time.
	done := make(chan struct{})
	subExp.Done().Return(done).Times(2)

	// When we tick over from the initial dispatch mode to
	// read mode, we will have the in channel assigned.
	deltas := make(chan []changestream.ChangeEvent)
	subExp.Changes().Return(deltas)
	subExp.Kill()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{opt: changestream.Namespace(
			"random_namespace",
			changestream.All,
		)},
	).Return(s.sub, nil)

	// The EventQueue is mocked, but we use a real Sqlite DB from which the
	// initial state is read. Insert some data to verify.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "CREATE TABLE random_namespace (key_name TEXT NOT NULL PRIMARY KEY)"); err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, "INSERT INTO random_namespace(key_name) VALUES ('some-key')")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	w, err := NewNamespaceWatcher(
		s.newBaseWatcher(c), InitialNamespaceChanges("SELECT key_name FROM random_namespace"),
		NamespaceFilter("random_namespace", changestream.All),
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case changes := <-w.Changes():
		c.Assert(changes, tc.HasLen, 1)
		c.Check(changes[0], tc.Equals, "some-key")
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for initial watcher changes")
	}

	workertest.CleanKill(c, w)
}

func (s *namespaceSuite) TestInitialStateSentByMapper(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subExp := s.sub.EXPECT()

	// We go through the worker loop twice; once to dispatch initial events,
	// then to pick up tomb.Dying(). Done() is read each time.
	done := make(chan struct{})
	subExp.Done().Return(done).Times(2)

	// When we tick over from the initial dispatch mode to
	// read mode, we will have the in channel assigned.
	deltas := make(chan []changestream.ChangeEvent)
	subExp.Changes().Return(deltas)
	subExp.Kill()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace(
			"random_namespace",
			changestream.All,
		)},
	).Return(s.sub, nil)

	// The EventQueue is mocked, but we use a real Sqlite DB from which the
	// initial state is read. Insert some data to verify.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "CREATE TABLE random_namespace (key_name TEXT NOT NULL PRIMARY KEY)"); err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, "INSERT INTO random_namespace(key_name) VALUES ('some-key')")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Notice that even if the mapper returns an empty list of change events,
	// the initial state is still sent. This is a hard requirement of the API.

	w, err := NewNamespaceMapperWatcher(
		s.newBaseWatcher(c), InitialNamespaceChanges("SELECT key_name FROM random_namespace"),
		func(ctx context.Context, e []changestream.ChangeEvent) ([]string, error) {
			return nil, nil
		},
		NamespaceFilter("random_namespace", changestream.All),
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case changes := <-w.Changes():
		c.Assert(changes, tc.HasLen, 1)
		c.Check(changes[0], tc.Equals, "some-key")
	case <-time.After(time.Second):
		c.Fatal("timed out waiting for initial watcher changes")
	}

	workertest.CleanKill(c, w)
}

func (s *namespaceSuite) TestDeltasSent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subExp := s.sub.EXPECT()

	// We go through the worker loop 4 times:
	// - Dispatch initial events.
	// - Read deltas.
	// - Dispatch deltas.
	// - Pick up tomb.Dying()
	done := make(chan struct{})
	subExp.Done().Return(done).Times(4)

	// Tick-tock-tick-tock. 2 assignments of the in channel.
	deltas := make(chan []changestream.ChangeEvent)
	subExp.Changes().Return(deltas).Times(2)

	subExp.Kill()

	// The specific table doesn't matter here. Only that exists to read from.
	// We don't need any initial data.
	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{opt: changestream.Namespace(
			"external_controller",
			changestream.All,
		)},
	).Return(s.sub, nil)

	w, err := NewNamespaceWatcher(
		s.newBaseWatcher(c), InitialNamespaceChanges("SELECT uuid FROM external_controller"),
		NamespaceFilter("external_controller", changestream.All),
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// No initial data.
	select {
	case changes := <-w.Changes():
		c.Assert(changes, tc.HasLen, 0)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for initial watcher changes")
	}

	select {
	case deltas <- []changestream.ChangeEvent{changeEvent{
		changeType: 0,
		namespace:  "external_controller",
		changed:    "some-ec-uuid",
	}}:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out dispatching change event")
	}

	select {
	case changes := <-w.Changes():
		c.Assert(changes, tc.HasLen, 1)
		c.Check(changes[0], tc.Equals, "some-ec-uuid")
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for watcher delta")
	}

	workertest.CleanKill(c, w)
}

func (s *namespaceSuite) TestDeltasSentByMapper(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subExp := s.sub.EXPECT()

	// We go through the worker loop 4 times:
	// - Dispatch initial events.
	// - Read deltas.
	// - Dispatch deltas.
	// - Read deltas. -- notice no dispatch because of the mapper.
	// - Pick up tomb.Dying()
	done := make(chan struct{})
	subExp.Done().Return(done).Times(5)

	// Tick-tock-tick-tock. 2 assignments of the in channel.
	deltas := make(chan []changestream.ChangeEvent)
	subExp.Changes().Return(deltas).Times(2)

	subExp.Kill()

	// The specific table doesn't matter here. Only that exists to read from.
	// We don't need any initial data.
	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace(
			"external_controller",
			changestream.All,
		)},
	).Return(s.sub, nil)

	w, err := NewNamespaceMapperWatcher(
		s.newBaseWatcher(c), InitialNamespaceChanges("SELECT uuid FROM external_controller"),
		func(ctx context.Context, e []changestream.ChangeEvent) ([]string, error) {
			if s := e[0].Changed(); s == "some-ec-uuid" {
				return singleton(s), nil
			}
			return nil, nil
		},
		NamespaceFilter("external_controller", changestream.All),
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// No initial data.
	select {
	case changes := <-w.Changes():
		c.Assert(changes, tc.HasLen, 0)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for initial watcher changes")
	}

	select {
	case deltas <- []changestream.ChangeEvent{changeEvent{
		changeType: 0,
		namespace:  "external_controller",
		changed:    "some-ec-uuid",
	}}:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out dispatching change event")
	}

	select {
	case changes := <-w.Changes():
		c.Assert(changes, tc.HasLen, 1)
		c.Check(changes[0], tc.Equals, "some-ec-uuid")
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for watcher delta")
	}

	select {
	case deltas <- []changestream.ChangeEvent{changeEvent{
		changeType: 0,
		namespace:  "external_controller",
		changed:    "some-other-uuid",
	}}:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out dispatching change event")
	}

	select {
	case <-w.Changes():
		c.Fatal("unexpected changes")
	case <-time.After(time.Second):
	}

	workertest.CleanKill(c, w)
}

func (s *namespaceSuite) TestDeltasSentByMapperError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subExp := s.sub.EXPECT()

	done := make(chan struct{})
	subExp.Done().Return(done).MinTimes(1)

	// Tick-tock-tick-tock. 2 assignments of the in channel.
	deltas := make(chan []changestream.ChangeEvent)
	subExp.Changes().Return(deltas).AnyTimes()

	subExp.Kill()

	// The specific table doesn't matter here. Only that exists to read from.
	// We don't need any initial data.
	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace(
			"external_controller",
			changestream.All,
		)},
	).Return(s.sub, nil)

	w, err := NewNamespaceMapperWatcher(
		s.newBaseWatcher(c), InitialNamespaceChanges("SELECT uuid FROM external_controller"),
		func(ctx context.Context, e []changestream.ChangeEvent) ([]string, error) {
			return nil, errors.New("boom")
		},
		NamespaceFilter("external_controller", changestream.All),
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// No initial data.
	select {
	case changes := <-w.Changes():
		c.Assert(changes, tc.HasLen, 0)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for initial watcher changes")
	}

	select {
	case deltas <- []changestream.ChangeEvent{changeEvent{
		changeType: 0,
		namespace:  "external_controller",
		changed:    "some-ec-uuid",
	}}:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out dispatching change event")
	}

	select {
	case _, ok := <-w.Changes():
		// Ensure the channel is closed, when the mapper dies.
		c.Assert(ok, tc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for watcher delta")
	}

	err = workertest.CheckKill(c, w)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *namespaceSuite) TestSubscriptionDoneKillsWorker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subExp := s.sub.EXPECT()

	done := make(chan struct{})
	close(done)
	subExp.Done().Return(done)

	subExp.Kill()

	// The specific table doesn't matter here. Only that exists to read from.
	// We don't need any initial data.
	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{opt: changestream.Namespace(
			"external_controller",
			changestream.All,
		)},
	).Return(s.sub, nil)

	w, err := NewNamespaceWatcher(
		s.newBaseWatcher(c), InitialNamespaceChanges("SELECT uuid FROM external_controller"),
		NamespaceFilter("external_controller", changestream.All),
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorIs, ErrSubscriptionClosed)
}

func (s *namespaceSuite) TestNilOption(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewNamespaceWatcher(
		s.newBaseWatcher(c),
		InitialNamespaceChanges("SELECT uuid FROM external_controller"),
		nil,
	)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *namespaceSuite) TestNilPredicate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewNamespaceWatcher(
		s.newBaseWatcher(c),
		InitialNamespaceChanges("SELECT uuid FROM external_controller"),
		PredicateFilter("random_namespace", changestream.All, nil),
	)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

type schemaDDLApplier struct{}

func (schemaDDLApplier) Apply(c *tc.C, ctx context.Context, runner database.TxnRunner) {
	schema := schema.New(
		schema.MakePatch(`
CREATE TABLE external_controller (
	uuid            TEXT NOT NULL PRIMARY KEY,
	alias           TEXT,
	ca_cert         TEXT NOT NULL
);
		`),
	)
	_, err := schema.Ensure(ctx, runner)
	c.Assert(err, tc.ErrorIsNil)
}
