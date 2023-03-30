// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stream

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
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

const (
	// PollInterval is the amount of time to wait between polling the database
	// for new stream events.
	PollInterval = time.Millisecond * 100
)

// Stream defines a worker that will poll the database for change events.
type Stream struct {
	tomb tomb.Tomb

	db           coredatabase.TrackedDB
	fileNotifier FileNotifier
	clock        clock.Clock
	logger       Logger

	changes chan changestream.ChangeEvent
	lastID  int64
}

// New creates a new Stream.
func New(db coredatabase.TrackedDB, fileNotifier FileNotifier, clock clock.Clock, logger Logger) *Stream {
	stream := &Stream{
		db:           db,
		fileNotifier: fileNotifier,
		clock:        clock,
		logger:       logger,
		changes:      make(chan changestream.ChangeEvent),
	}

	stream.tomb.Go(stream.loop)

	return stream
}

// Changes returns a channel for a given namespace (database).
// The channel will return events represented by change log rows
// from the database.
// The change event IDs will be monotonically increasing
// (though not necessarily sequential).
// Events will be coalesced into a single change if they are
// for the same entity and edit type.
func (s *Stream) Changes() <-chan changestream.ChangeEvent {
	return s.changes
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

	timer := s.clock.NewTimer(PollInterval)
	defer timer.Stop()

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

		case <-timer.Chan():
			changes, err := s.readChanges()
			if err != nil {
				// If we get an error attempting to read the changes, the Txn
				// will have retried multiple times. There just isn't anything
				// we can do, so we just return an error.
				return errors.Annotate(err, "reading change")
			}

			for _, change := range changes {
				if s.logger.IsTraceEnabled() {
					s.logger.Tracef("change event: %v", change)
				}
				s.changes <- change
				s.lastID = change.id
			}

			// TODO (stickupkid): Adaptive polling based on the number of
			// changes that are returned.
			timer.Reset(PollInterval)
		}
	}
}

const (
	query = `
SELECT MAX(c.id), c.edit_type_id, n.namespace, changed_uuid, created_at
	FROM change_log c
		JOIN change_log_edit_type t ON c.edit_type_id = t.id
		JOIN change_log_namespace n ON c.namespace_id = n.id
	WHERE c.id > ?
	GROUP BY c.edit_type_id, c.namespace_id, c.changed_uuid 
	ORDER BY c.id DESC;
`
)

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
