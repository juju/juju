// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/changestream/eventmultiplexer"
	"github.com/juju/juju/internal/changestream/stream"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

const (
	// This is copied from the internal/changestream/stream/stream.go file.
	// This is so we don't expose the state name outside of the package.
	stateIdle = "idle"
)

// TestWatchableDB creates a watchable DB for running the ChangeStream
// implementation for use inside of tests. This doesn't use the dependency
// engine and creates a catacomb for managing the lifecycle of the
// components.
type TestWatchableDB struct {
	catacomb catacomb.Catacomb

	db     database.TxnRunner
	stream *stream.Stream
	mux    *eventmultiplexer.EventMultiplexer

	states chan string
}

// NewTestWatchableDB creates a test changestream based on the id and
// runnner.
func NewTestWatchableDB(c *tc.C, id string, db database.TxnRunner) *TestWatchableDB {
	states := make(chan string, 1)

	logger := loggertesting.WrapCheckLog(c)
	stream := stream.NewInternalStates(id, db, newNoopFileWatcher(), clock.WallClock, noopMetrics{}, logger, states)
	mux, err := eventmultiplexer.New(stream, clock.WallClock, noopMetrics{}, logger)
	c.Assert(err, tc.ErrorIsNil)

	h := TestWatchableDB{
		db:     db,
		stream: stream,
		mux:    mux,
		states: states,
	}

	err = catacomb.Invoke(catacomb.Plan{
		Name: "test-changestream",
		Site: &h.catacomb,
		Work: h.loop,
		Init: []worker.Worker{
			h.stream,
			h.mux,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	return &h
}

// Stream returns the underlying changestream stream implementation.
func (h *TestWatchableDB) Stream() *stream.Stream {
	return h.stream
}

// EventMultiplexer returns the underlying event multiplixer from
// the change stream implementation.
func (h *TestWatchableDB) EventMultiplexer() *eventmultiplexer.EventMultiplexer {
	return h.mux
}

// Txn manages the application of a SQLair transaction within which the
// input function is executed. See https://github.com/canonical/sqlair.
// The input context can be used by the caller to cancel this process.
func (w *TestWatchableDB) Txn(ctx context.Context, fn func(context.Context, *sqlair.TX) error) error {
	return w.db.Txn(ctx, fn)
}

// StdTxn manages the application of a standard library transaction within
// which the input function is executed.
// The input context can be used by the caller to cancel this process.
func (w *TestWatchableDB) StdTxn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return w.db.StdTxn(ctx, fn)
}

// EventSource returns the event source for this worker.
func (w *TestWatchableDB) Subscribe(opts ...changestream.SubscriptionOption) (changestream.Subscription, error) {
	return w.mux.Subscribe(opts...)
}

// Kill stops the test change stream.
func (h *TestWatchableDB) Kill() {
	h.catacomb.Kill(nil)
}

// Wait waits for the test change stream.
func (h *TestWatchableDB) Wait() error {
	return h.catacomb.Err()
}

func (h *TestWatchableDB) loop() error {
	<-h.catacomb.Dying()
	return h.catacomb.ErrDying()
}

type noopFileWatcher struct {
	tomb tomb.Tomb
	ch   chan bool
}

func newNoopFileWatcher() *noopFileWatcher {
	w := &noopFileWatcher{
		ch: make(chan bool),
	}

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})

	return w
}

func (w *noopFileWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *noopFileWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *noopFileWatcher) Changes() (<-chan bool, error) {
	return w.ch, nil
}

type noopMetrics struct{}

func (noopMetrics) WatermarkInsertsInc()                             {}
func (noopMetrics) WatermarkRetriesInc()                             {}
func (noopMetrics) ChangesRequestDurationObserve(val float64)        {}
func (noopMetrics) ChangesCountObserve(val int)                      {}
func (noopMetrics) SubscriptionsInc()                                {}
func (noopMetrics) SubscriptionsDec()                                {}
func (noopMetrics) DispatchDurationObserve(val float64, failed bool) {}
