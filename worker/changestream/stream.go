// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/changestream"
	coredb "github.com/juju/juju/core/db"
)

const (
	// PollInterval is the amount of time to wait between polling the database
	// for new stream events.
	PollInterval = time.Millisecond * 100
)

// Stream defines a worker that will poll the database for change events.
type Stream struct {
	catacomb catacomb.Catacomb

	db           coredb.TrackedDB
	fileNotifier FileNotifier
	clock        clock.Clock
	logger       Logger

	changes chan changestream.ChangeEvent
	lastID  int64
}

// NewStream creates a new Stream.
func NewStream(db coredb.TrackedDB, fileNotifier FileNotifier, clock clock.Clock, logger Logger) (DBStream, error) {
	stream := &Stream{
		db:           db,
		fileNotifier: fileNotifier,
		clock:        clock,
		logger:       logger,
		changes:      make(chan changestream.ChangeEvent),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &stream.catacomb,
		Work: stream.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return stream, nil
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
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Stream) Wait() error {
	return w.catacomb.Wait()
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
		case <-s.catacomb.Dying():
			return s.catacomb.ErrDying()

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
			// receive a change event from the file notifier.
		INNER:
			for {
				select {
				case <-s.catacomb.Dying():
					return s.catacomb.ErrDying()
				case inner, ok := <-fileNotifier:
					if !ok || !inner {
						break INNER
					}
					s.logger.Debugf("ignoring file change event")
				}
			}

			s.logger.Infof("Change stream has been unblocked")

		case <-timer.Chan():
			changes, err := s.readChanges()
			if err != nil {
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
	var changes []changeEvent
	err := s.db.Txn(context.TODO(), func(ctx context.Context, tx *sql.Tx) error {
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
