// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stream

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
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

var (
	// The backoff strategy is used to back-off when we get no changes
	// from the database. This is used to prevent the worker from polling
	// the database too frequently and allow us to attempt to coalesce
	// changes when there is less activity.
	backOffStrategy = retry.ExpBackoff(time.Millisecond*10, time.Millisecond*250, 1.5, false)
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
	IsTraceEnabled() bool
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

type reportRequest struct {
	data map[string]any
	done chan struct{}
}

// Stream defines a worker that will poll the database for change events.
type Stream struct {
	tomb tomb.Tomb

	tag          string
	db           coredatabase.TxnRunner
	fileNotifier FileNotifier
	clock        clock.Clock
	logger       Logger

	terms chan changestream.Term

	idMutex                sync.Mutex
	lastID, recordedLastID int64
}

// New creates a new Stream.
func New(tag string, db coredatabase.TxnRunner, fileNotifier FileNotifier, clock clock.Clock, logger Logger) *Stream {
	stream := &Stream{
		tag:            tag,
		db:             db,
		fileNotifier:   fileNotifier,
		clock:          clock,
		logger:         logger,
		terms:          make(chan changestream.Term),
		recordedLastID: -1,
	}

	stream.tomb.Go(stream.loop)

	return stream
}

// Report returns
func (s *Stream) Report() map[string]any {
	s.idMutex.Lock()
	defer s.idMutex.Unlock()

	return map[string]any{
		"last-id":          s.lastID,
		"recorded-last-id": s.recordedLastID,
	}
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

// Kill is part of the worker.Worker interface.
func (w *Stream) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Stream) Wait() error {
	return w.tomb.Wait()
}

func (s *Stream) loop() error {
	watermarkTimer := s.clock.NewTimer(defaultWatermarkInterval)
	defer watermarkTimer.Stop()

	fileNotifier, err := s.fileNotifier.Changes()
	if err != nil {
		return errors.Annotate(err, "getting file notifier")
	}

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

			s.logger.Infof("Change stream has been blocked")

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
					s.logger.Debugf("ignoring file change event")
				}
			}

			s.logger.Infof("Change stream has been unblocked")

		case <-watermarkTimer.Chan():
			// Every interval we'll write the last ID to the database. This
			// allows us to prune the change files that are no longer needed.
			// This is a best effort, so if we fail to write the last ID, we
			// just continue. As long as at least one write happens in the
			// time between now and the pruning of the change log.
			// The addition of a witness_at timestamp allows us to see how
			// far each controller is behind the current time.
			if err := s.recordWatermark(); err != nil {
				s.logger.Infof("failed to record last ID: %v", err)
			}

			watermarkTimer.Reset(defaultWatermarkInterval)

		default:
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

			if len(changes) == 0 {
				// The following uses the back-off strategy for polling the
				// database for changes. If we repeatedly get no changes, then
				// we back=off exponentially. This should prevent us from
				// stuttering and should allow use to coalesce in the large
				// case when nothing is happening.
				attempt++
				select {
				case <-s.tomb.Dying():
					return tomb.ErrDying
				case <-s.clock.After(backOffStrategy(0, attempt)):
					continue
				}
			}

			// Term encapsulates a set of changes that are bounded by a
			// coalesced set.
			term := &Term{
				done: make(chan bool),
			}

			var lastID int64
			traceEnabled := s.logger.IsTraceEnabled()
			for _, change := range changes {
				if traceEnabled {
					s.logger.Tracef("change event: %v", change)
				}
				term.changes = append(term.changes, change)
				lastID = change.id
			}

			// Send the term to the terms channel, and wait for it to be
			// completed. This will block the outer loop until the term has
			// been completed. It is the responsibility of the consumer of the
			// terms channel to ensure that the term is completed in the
			// fastest possible time.
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
					s.logger.Infof("term has been aborted")
					continue
				}

				// If the resulting term change set is empty, then wait for
				// the back-off strategy to complete before attempting to
				// read changes again.
				// This is to prevent the worker from polling the database
				// too frequently and allow us to attempt to coalesce changes
				// when there is less activity.
				if empty {
					attempt++
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

				// Only when the term is completed, do we update the last ID
				// for the watermark. This ensures that all changes are read
				// and processed from the term and that we don't prematurely
				// update the watermark.
				s.idMutex.Lock()
				s.lastID = lastID
				s.idMutex.Unlock()
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
SELECT MAX(c.id), c.edit_type_id, n.namespace, changed_uuid, created_at
	FROM change_log c
		JOIN change_log_edit_type t ON c.edit_type_id = t.id
		JOIN change_log_namespace n ON c.namespace_id = n.id
	WHERE c.id > ?
	GROUP BY c.namespace_id, c.changed_uuid 
	ORDER BY c.id;
`
)

// Note: changestream.ChangeEvent could be optimized in the future to be a
// struct instead of an interface. We should work out if this is a good idea
// or not.
type changeEvent struct {
	id          int64
	changeType  int
	namespace   string
	changedUUID string
	createdAt   string
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

// ChangedUUID returns the entity UUID of the change.
func (e changeEvent) ChangedUUID() string {
	return e.changedUUID
}

func (s *Stream) readChanges() ([]changeEvent, error) {
	// As this is a self instantiated query, we don't have a root context to tie
	// to, so we create a new one that's cancellable.
	ctx, cancel := s.scopedContext()
	defer cancel()

	var changes []changeEvent
	err := s.db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, selectQuery, s.lastID)
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
				&changes[i].changedUUID,
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
	// Attempt insert the watermark for a given controller. If the controller
	// already exists, then update the change log ID and the witness time, only
	// if the change log ID is greater than the current change log ID.
	watermarkQuery = `
INSERT INTO change_log_witness (tag, change_log_id, last_seen_at) VALUES (?, ?, datetime())
	ON CONFLICT (tag) DO UPDATE SET 
		change_log_id = EXCLUDED.change_log_id, 
		last_seen_at = EXCLUDED.last_seen_at
	WHERE EXCLUDED.change_log_id > change_log_witness.change_log_id;
`
)

// recordWatermark records the last ID that was processed. This is used to
// ensure that we can prune the change log table.
func (s *Stream) recordWatermark() error {
	// We only need to record the watermark if it has changed. This should
	// prevent unnecessary writes to the database, when we know that nothing
	// has changed.
	s.idMutex.Lock()
	if s.lastID == s.recordedLastID {
		s.idMutex.Unlock()
		return nil
	}
	s.idMutex.Unlock()

	ctx, cancel := s.scopedContext()
	defer cancel()

	return s.db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, watermarkQuery, s.tag, s.lastID)
		if err != nil {
			return errors.Annotate(err, "recording watermark")
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return errors.Annotate(err, "recording watermark")
		}
		if affected != 1 {
			return errors.Errorf("expected to affect 1 row, affected %d", affected)
		}

		s.idMutex.Lock()
		s.recordedLastID = s.lastID
		s.idMutex.Unlock()

		return nil
	})
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *Stream) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.tomb.Context(ctx), cancel
}
