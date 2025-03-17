// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stream

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/database"
)

const (
	// defaultWaitTermTimeout is the default timeout for waiting for a term
	// to be completed. If the term is not completed within this time, then
	// the worker will return an error and restart.
	defaultWaitTermTimeout = time.Second * 30

	// defaultWatermarkInterval is the default interval to wait before
	// updating the watermark.
	defaultWatermarkInterval = 5 * time.Second
)

const (
	// States which report the state of the worker.
	stateIdle = "idle"
)

var (
	// The backoff strategy is used to back-off when we get no changes
	// from the database. This is used to prevent the worker from polling
	// the database too frequently and allow us to attempt to coalesce
	// changes when there is less activity.
	backOffStrategy = retry.ExpBackoff(time.Millisecond*100, time.Second*10, 1.4, false)
)

// MetricsCollector represents the metrics methods called.
type MetricsCollector interface {
	WatermarkInsertsInc()
	WatermarkRetriesInc()
	ChangesRequestDurationObserve(val float64)
	ChangesCountObserve(val int)
}

// FileNotifyWatcher notifies when a file has been created or deleted within
// a given directory.
type FileNotifier interface {
	// Changes returns a channel if a file was created or deleted.
	Changes() (<-chan bool, error)
}

// Term represents a set of changes that are bounded by a coalesced set.
// The notion of a term are a set of changes that can be run one at a time
// asynchronously. Allowing changes within a given term to be signaled of a
// change independently from one another.
// Once a change within a term has been completed, only at that point
// is another change processed, until all changes are exhausted.
type Term struct {
	changes []changestream.ChangeEvent
	done    chan bool
}

// Changes returns the changes that are part of the term.
func (t *Term) Changes() []changestream.ChangeEvent {
	return t.changes
}

// Done signals that the term has been completed.
func (t *Term) Done(empty bool, abort <-chan struct{}) {
	select {
	case t.done <- empty:
	case <-abort:
		// We need to signal that the term has been aborted, so we don't
		// block the change stream.
		close(t.done)
	}
}

// termView represents a time window of change log IDs for a given term.
type termView struct {
	lower, upper int64
}

// Equals returns true if the termView is equal to the other termView.
func (v *termView) Equals(other *termView) bool {
	return v.lower == other.lower && v.upper == other.upper
}

// String returns the string representation of the termView.
func (v *termView) String() string {
	return fmt.Sprintf("(lower: %d, upper: %d)", v.lower, v.upper)
}

// Stream defines a worker that will poll the database for change events.
type Stream struct {
	internalStates chan string
	tomb           tomb.Tomb

	id           string
	db           coredatabase.TxnRunner
	fileNotifier FileNotifier
	clock        clock.Clock
	logger       logger.Logger
	metrics      MetricsCollector

	terms chan changestream.Term

	watermarksMutex       sync.Mutex
	watermarks            []*termView
	lastRecordedWatermark *termView
}

// New creates a new Stream.
func New(
	id string,
	db coredatabase.TxnRunner,
	fileNotifier FileNotifier,
	clock clock.Clock,
	metrics MetricsCollector,
	logger logger.Logger,
) *Stream {
	return NewInternalStates(id, db, fileNotifier, clock, metrics, logger, nil)
}

// NewInternalStates creates a new Stream with an internal state channel.
func NewInternalStates(
	id string,
	db coredatabase.TxnRunner,
	fileNotifier FileNotifier,
	clock clock.Clock,
	metrics MetricsCollector,
	logger logger.Logger,
	internalStates chan string,
) *Stream {
	stream := &Stream{
		id:             id,
		db:             db,
		fileNotifier:   fileNotifier,
		clock:          clock,
		logger:         logger,
		metrics:        metrics,
		terms:          make(chan changestream.Term),
		watermarks:     make([]*termView, changestream.DefaultNumTermWatermarks),
		internalStates: internalStates,
	}

	stream.tomb.Go(stream.loop)

	return stream
}

// Report returns
func (s *Stream) Report() map[string]any {
	s.watermarksMutex.Lock()
	defer s.watermarksMutex.Unlock()

	m := map[string]any{
		"id":                      s.id,
		"last-recorded-watermark": "",
	}

	if s.lastRecordedWatermark != nil {
		m["last-recorded-watermark"] = s.lastRecordedWatermark.String()
	}

	termViews := make([]string, 0)
	for _, termView := range s.watermarks {
		if termView == nil {
			continue
		}
		termViews = append(termViews, termView.String())
	}
	m["watermarks"] = strings.Join(termViews, ", ")

	return m
}

// Terms returns a channel for a given namespace (database) that returns
// a set of terms. The notion of terms are a set of changes that can be
// run one at a time asynchronously. Allowing changes within a given
// term to be signaled of a change independently from one another.
// Once a change within a term has been completed, only at that point
// is another change processed, until all changes are exhausted.
func (s *Stream) Terms() <-chan changestream.Term {
	return s.terms
}

// Dying returns a channel to notify when the stream is dying.
func (s *Stream) Dying() <-chan struct{} {
	return s.tomb.Dying()
}

// Kill is part of the worker.Worker interface.
func (s *Stream) Kill() {
	s.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (s *Stream) Wait() error {
	return s.tomb.Wait()
}

func (s *Stream) loop() error {
	watermarkTimer := s.clock.NewTimer(defaultWatermarkInterval)
	defer watermarkTimer.Stop()

	fileNotifier, err := s.fileNotifier.Changes()
	if err != nil {
		return errors.Annotate(err, "getting file notifier")
	}

	// Insert the initial watermark into the table for the given id.
	if err := s.createWatermark(); err != nil {
		return errors.Trace(err)
	}

	ctx, cancel := s.scopedContext()
	defer cancel()

	var attempt int
	for {
		select {
		case <-s.tomb.Dying():
			return tomb.ErrDying

		case fileCreated, ok := <-fileNotifier:
			if !ok {
				fileNotifier, err = s.fileNotifier.Changes()
				if err != nil {
					return errors.Annotate(err, "retrying file notifier")
				}
				continue
			}

			// If the file was removed, just continue, we're clearly not in the
			// middle of a created block.
			if !fileCreated {
				continue
			}

			s.logger.Infof(ctx, "Change stream has been blocked")

			// Create an inner loop, that will block the outer loop until we
			// receive a change fileCreated event from the file notifier.
		INNER:
			for {
				select {
				case <-s.tomb.Dying():
					return tomb.ErrDying
				case inner, ok := <-fileNotifier:
					if !ok || !inner {
						break INNER
					}
					// If we get a fileCreated event, and we're already waiting
					// for a fileRemoved event, then we're in the middle of a
					// block, so just continue.
					s.logger.Debugf(ctx, "ignoring file change event")
				}
			}

			s.logger.Infof(ctx, "Change stream has been unblocked")

		case <-watermarkTimer.Chan():
			// Every interval we'll write the last ID to the database. This
			// allows us to prune the change files that are no longer needed.
			// This is a best effort, so if we fail to write the last ID, we
			// just continue. As long as at least one write happens in the
			// time between now and the pruning of the change log.
			// The addition of a witness_at timestamp allows us to see how
			// far each controller is behind the current time.
			if err := s.updateWatermark(); errors.Is(err, coredatabase.ErrDBDead) {
				// If the database is dead, then we should just return and
				// let the worker die.
				return errors.Trace(err)
			} else if err != nil {
				s.logger.Infof(ctx, "failed to record last ID: %v", err)
			}

			// Jitter the watermark interval to prevent all workers from
			// polling the database at the same time.
			watermarkTimer.Reset(jitter(defaultWatermarkInterval, 0.1))

		default:
			begin := s.clock.Now()
			changes, err := s.readChanges()
			if err != nil {
				// If the context was canceled, we're unsure if it's because
				// the worker is dying, or if the context was canceled because
				// the db was slow. In any case, continue and let the worker
				// die if it's dying.
				if errors.Is(errors.Cause(err), context.Canceled) {
					continue
				}
				// If we get an error attempting to read the changes, the Txn
				// will have retried multiple times. There just isn't anything
				// we can do, so we just return an error.
				return errors.Annotate(err, "reading change")
			}

			traceEnabled := s.logger.IsLevelEnabled(logger.TRACE)
			if traceEnabled {
				s.logger.Tracef(ctx, "read %d changes", len(changes))
			}

			// We only want to record successful changes retrieve
			// queries on the metrics.
			s.metrics.ChangesRequestDurationObserve(s.clock.Now().Sub(begin).Seconds())

			if len(changes) == 0 {
				// The following uses the back-off strategy for polling the
				// database for changes. If we repeatedly get no changes, then
				// we back=off exponentially. This should prevent us from
				// stuttering and should allow use to coalesce in the large
				// case when nothing is happening.
				attempt++

				if traceEnabled {
					s.logger.Tracef(ctx, "no changes, with attempt %d", attempt)
				}

				select {
				case <-s.tomb.Dying():
					return tomb.ErrDying
				case <-s.clock.After(backOffStrategy(0, attempt)):
					if err := s.reportIdleState(attempt); err != nil {
						return errors.Trace(err)
					}
					continue
				}
			}

			// Record the number of retrieved changes on metrics.
			s.metrics.ChangesCountObserve(len(changes))
			var (
				// Term encapsulates a set of changes that are bounded by a
				// coalesced set.
				term = &Term{
					done: make(chan bool),
				}

				lower = int64(math.MaxInt64)
				upper = int64(math.MinInt64)
			)
			for _, change := range changes {
				if traceEnabled {
					s.logger.Tracef(ctx, "change event: %v", change)
				}
				term.changes = append(term.changes, change)

				if change.id < lower {
					lower = change.id
				}
				if change.id > upper {
					upper = change.id
				}
			}
			if lower == math.MaxInt64 || upper == math.MinInt64 {
				// This should never happen, but if it does, just continue.
				s.logger.Infof(ctx, "invalid lower or upper bound: lower: %d, upper: %d", lower, upper)
				continue
			}

			// Send the term to the terms channel, and wait for it to be
			// completed. This will block the outer loop until the term has
			// been completed. It is the responsibility of the consumer of the
			// terms channel to ensure that the term is completed in the
			// fastest possible time.

			if traceEnabled {
				s.logger.Tracef(ctx, "term start: processing changes %d", len(changes))
			}

			select {
			case <-s.tomb.Dying():
				return tomb.ErrDying
			case s.terms <- term:
			}

			select {
			case <-s.tomb.Dying():
				return tomb.ErrDying

			case <-s.clock.After(defaultWaitTermTimeout):
				// This is a critical error, we should never get here if juju
				// is humming along. This is a sign that something is wrong
				// with the dependencies of the worker. We have no choice but
				// to return an error and to bounce the world.
				return errors.Errorf("term has not been completed in time")

			case empty, ok := <-term.done:
				if !ok {
					// If the event mux has been killed, then the term has been
					// aborted, so we just continue. This is likely the case
					// when the worker is dying. We don't want to block the
					// change stream, so we just continue.
					s.logger.Infof(ctx, "term has been aborted")
					continue
				}

				// Only when the term is completed, do we update the lower
				// and upper bounds of the watermark. This ensures that all
				// changes are read and processed from the term and that we
				// don't prematurely update the watermark.
				s.recordTermView(&termView{
					lower: lower,
					upper: upper,
				})

				// If the resulting term change set is empty, then wait for
				// the back-off strategy to complete before attempting to
				// read changes again.
				// This is to prevent the worker from polling the database
				// too frequently and allow us to attempt to coalesce changes
				// when there is less activity.
				if empty {
					attempt++

					if traceEnabled {
						s.logger.Tracef(ctx, "empty term, with attempt %d", attempt)
					}

					select {
					case <-s.tomb.Dying():
						return tomb.ErrDying
					case <-s.clock.After(backOffStrategy(0, attempt)):
						continue
					}
				}

				// Reset the attempt counter if we get changes, so the
				// back=off strategy is reset.
				attempt = 0

				if traceEnabled {
					s.logger.Tracef(ctx, "term done: processed changes %d", len(changes))
				}
			}
		}
	}
}

const (
	// Ordering of create, update and delete are in that order, based on the
	// transactions are inserted in that order.
	// If the namespace is later deleted, you'll no longer locate that during
	// a select.
	selectQuery = `
SELECT MAX(c.id), c.edit_type_id, n.namespace, changed, created_at
	FROM change_log c
		JOIN change_log_edit_type t ON c.edit_type_id = t.id
		JOIN change_log_namespace n ON c.namespace_id = n.id
	WHERE c.id > ?
	GROUP BY c.namespace_id, c.changed
	ORDER BY c.id;
`
)

// Note: changestream.ChangeEvent could be optimized in the future to be a
// struct instead of an interface. We should work out if this is a good idea
// or not.
type changeEvent struct {
	id         int64
	changeType int
	namespace  string
	changed    string
	createdAt  string
}

// Type returns the type of change (create, update, delete).
func (e changeEvent) Type() changestream.ChangeType {
	return changestream.ChangeType(e.changeType)
}

// Namespace returns the namespace of the change. This is normally the
// table name.
func (e changeEvent) Namespace() string {
	return e.namespace
}

// Changed returns the changed value of event. This logically can be
// the primary key of the row that was changed or the field of the change
// that was changed.
func (e changeEvent) Changed() string {
	return e.changed
}

func (s *Stream) readChanges() ([]changeEvent, error) {
	// As this is a self instantiated query, we don't have a root context to tie
	// to, so we create a new one that's cancellable.
	ctx, cancel := s.scopedContext()
	defer cancel()

	var changes []changeEvent
	err := s.db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, selectQuery, s.upperBound())
		if err != nil {
			return errors.Annotate(err, "querying for changes")
		}
		defer rows.Close()

		dest := func(i int) []interface{} {
			changes = append(changes, changeEvent{})
			return []interface{}{
				&changes[i].id,
				&changes[i].changeType,
				&changes[i].namespace,
				&changes[i].changed,
				&changes[i].createdAt,
			}
		}
		for i := 0; rows.Next(); i++ {
			if err := rows.Scan(dest(i)...); err != nil {
				return errors.Annotate(err, "scanning change")
			}
		}
		return nil
	})
	return changes, errors.Trace(err)
}

const (
	watermarkCreateQuery = `
INSERT INTO change_log_witness
	(controller_id, lower_bound, upper_bound, updated_at)
VALUES
	(?, -1, -1, datetime())
ON CONFLICT (controller_id) DO NOTHING;
`
)

func (s *Stream) createWatermark() error {
	ctx, cancel := s.scopedContext()
	defer cancel()

	return s.db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, watermarkCreateQuery, s.id)
		if err != nil {
			if database.IsErrConstraintPrimaryKey(err) {
				return nil
			}
			return errors.Annotate(err, "recording watermark")
		}
		if _, err := result.RowsAffected(); err != nil {
			return errors.Annotate(err, "recording watermark")
		}
		return nil
	})
}

const (
	// Update the watermark for a given controller.
	watermarkUpdateQuery = `
UPDATE change_log_witness
SET
	lower_bound = ?,
	upper_bound = ?,
	updated_at = datetime()
WHERE controller_id = ?;
`
)

// updateWatermark records the last ID that was processed. This is used to
// ensure that we can prune the change log table.
func (s *Stream) updateWatermark() error {
	ctx, cancel := s.scopedContext()
	defer cancel()

	return s.db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Run this per transaction, so that we're using the latest lower bound
		// and upper bound.
		// We do this inside of the transaction that is retryable, so that we
		// don't end up blocking the updating of the term during the term
		// completion.
		return s.processWatermark(func(view *termView) error {
			result, err := tx.ExecContext(ctx, watermarkUpdateQuery, view.lower, view.upper, s.id)
			if err != nil {
				return errors.Annotate(err, "recording watermark")
			}

			if affected, err := result.RowsAffected(); err != nil {
				return errors.Annotate(err, "recording watermark")
			} else if affected != 1 {
				return errors.Errorf("expected 1 row to be affected, got %d", affected)
			}

			s.metrics.WatermarkInsertsInc()

			return nil
		})
	})
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (s *Stream) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(s.tomb.Context(context.Background()))
}

func (s *Stream) upperBound() int64 {
	s.watermarksMutex.Lock()
	defer s.watermarksMutex.Unlock()

	if len(s.watermarks) == 0 {
		if s.lastRecordedWatermark == nil {
			return -1
		}
		return s.lastRecordedWatermark.upper
	}

	tail := s.watermarks[len(s.watermarks)-1]
	if tail == nil {
		return -1
	}

	return tail.upper
}

func (s *Stream) recordTermView(v *termView) {
	s.watermarksMutex.Lock()
	defer s.watermarksMutex.Unlock()

	// Insert the latest termView into the watermark list.
	s.watermarks = append(s.watermarks, v)

	// To prevent unbounded growth of the watermark list, we prune the list
	// to the default number of term watermarks.
	// It is safe to do this, because the change log table is pruned at best
	// effort. We guarantee that the change log pruning will not prune any
	// changes that are still in the watermark list or are in the future,
	// because they've not actually been persisted to the database yet. They're
	// just in memory per worker.
	// If a watermark list termView is pruned from the front and it's not yet been
	// written to the change log table, then another write will be attempted
	// with a higher lower bound. The pruner will not prune the change log
	// table until the lower bound is greater than the lower bound of the
	// change log table.
	// We can only guarantee this because the change log id is monotonically
	// increasing. Pruning will only ever see a higher number, missing writes
	// to the witness table will just see a lower bound number from the pruner
	// perspective. Once a write is made, the pruner will just remove the lower
	// bound from the witness tables.
	if num := len(s.watermarks); num > changestream.DefaultNumTermWatermarks {
		s.watermarks = s.watermarks[num-changestream.DefaultNumTermWatermarks:]
	}
}

// processWatermark runs the given function on the head of the watermark list.
// If the function succeeds, then the head of the watermark list is removed.
// If the function fails, then the watermark list is not modified and the
// error is returned.
// Note: this acts like a transaction, either if succeeds or it doesn't.
func (s *Stream) processWatermark(fn func(*termView) error) error {
	s.watermarksMutex.Lock()
	defer s.watermarksMutex.Unlock()

	// Here we only record the retried metrics because this function is
	// wrapped in a retriable transaction.
	s.metrics.WatermarkRetriesInc()

	// Nothing to do if there are no watermarks.
	if len(s.watermarks) < changestream.DefaultNumTermWatermarks {
		return nil
	}

	// If the buffer isn't full, then we don't need to process the watermark.
	head := s.watermarks[0]
	if head == nil {
		return nil
	}

	// Run the function on the head of the watermark list.
	if err := fn(head); err != nil {
		return errors.Trace(err)
	}

	// If that succeeded, then we can remove the head of the watermark list.
	s.watermarks = s.watermarks[1:]
	s.lastRecordedWatermark = head
	return nil
}

// reportIdleState reports the idle state to the internal states channel.
func (s *Stream) reportIdleState(attempt int) error {
	// If there are no internal states, then we don't need to report the idle
	// state. This is only used for testing.
	if s.internalStates == nil {
		return nil
	}

	maxChangeLogID, err := s.latestChangeLogID()
	if err != nil {
		return errors.Annotate(err, "getting latest change log ID")
	}

	if bound := s.upperBound(); bound > 0 && maxChangeLogID > 0 && bound == maxChangeLogID {
		s.logger.Tracef(context.TODO(), "no changes, backing off after %d attempt", attempt)

		// Report the idle state to the internal states channel.
		select {
		case <-s.tomb.Dying():
		case s.internalStates <- stateIdle:
		default:
		}
	}

	return nil
}

// latestChangeLogID returns the latest change log ID and is used to determine
// if the worker is idle.
func (s *Stream) latestChangeLogID() (int64, error) {
	ctx, cancel := s.scopedContext()
	defer cancel()

	var id int64
	err := s.db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, "SELECT IFNULL(MAX(id), -1) FROM change_log")
		if err := row.Scan(&id); err != nil {
			return errors.Annotate(err, "getting latest change log ID")
		}
		return nil
	})
	return id, errors.Trace(err)
}

// jitter returns a duration that is the input interval with a random factor
// applied to it. This prevents all workers from polling the database at the
// same time.
func jitter(interval time.Duration, factor float64) time.Duration {
	return time.Duration(float64(interval) * (1 + factor*(2*rand.Float64()-1)))
}
