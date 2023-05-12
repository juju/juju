// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stream

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
)

// Logger represents the logging methods called.
type Logger interface {
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
	IsTraceEnabled() bool
}

// FileNotifyWatcher represents a way to watch for changes in a namespace folder
// directory.
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
	done    chan struct{}
}

// Changes returns the changes that are part of the term.
func (t *Term) Changes() []changestream.ChangeEvent {
	return t.changes
}

// Done signals that the term has been completed.
func (t *Term) Done() {
	close(t.done)
}

// Stream defines a worker that will poll the database for change events.
type Stream struct {
	tomb tomb.Tomb

	db           coredatabase.TrackedDB
	fileNotifier FileNotifier
	clock        clock.Clock
	logger       Logger

	terms  chan changestream.Term
	lastID int64
}

// New creates a new Stream.
func New(db coredatabase.TrackedDB, fileNotifier FileNotifier, clock clock.Clock, logger Logger) *Stream {
	stream := &Stream{
		db:           db,
		fileNotifier: fileNotifier,
		clock:        clock,
		logger:       logger,
		terms:        make(chan changestream.Term),
	}

	stream.tomb.Go(stream.loop)

	return stream
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
	fileNotifier, err := s.fileNotifier.Changes()
	if err != nil {
		return errors.Annotate(err, "getting file notifier")
	}

	var (
		attempt         int
		backOffStrategy = retry.ExpBackoff(time.Millisecond*10, time.Millisecond*250, 1.5, true)
	)
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
				}
				continue
			}

			// Reset the attempt counter if we get changes, so the back=off
			// strategy is reset.
			attempt = 0

			// Term encapsulates a set of changes that are bounded by a
			// coalesced set.
			term := &Term{
				done: make(chan struct{}),
			}

			traceEnabled := s.logger.IsTraceEnabled()
			for _, change := range changes {
				if traceEnabled {
					s.logger.Tracef("change event: %v", change)
				}
				term.changes = append(term.changes, change)
				s.lastID = change.id
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
			case <-term.done:
			}
		}
	}
}

const (
	// Ordering of create, update and delete are in that order, based on the
	// transactions are inserted in that order.
	// If the namespace is later deleted, you'll no longer locate that during
	// a select.
	query = `
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
	err := s.db.Txn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query, s.lastID)
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

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *Stream) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.tomb.Context(ctx), cancel
}
