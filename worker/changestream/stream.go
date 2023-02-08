package changestream

import (
	"database/sql"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/database"
	"gopkg.in/tomb.v2"
)

const (
	// PollInterval is the amount of time to wait between polling the database
	// for new stream events.
	PollInterval = time.Millisecond * 100
)

// Stream defines a worker that will poll the database for change events.
type Stream struct {
	db     *sql.DB
	clock  clock.Clock
	logger Logger

	changes chan changestream.ChangeEvent
	lastID  int64

	tomb tomb.Tomb
}

// NewStream creates a new Stream.
func NewStream(db *sql.DB, clock clock.Clock, logger Logger) DBStream {
	stream := &Stream{
		db:      db,
		clock:   clock,
		logger:  logger,
		changes: make(chan changestream.ChangeEvent),
	}

	stream.tomb.Go(stream.loop)

	return stream
}

// Changes returns a channel that will contain new changes events. The channel
// will return change events that represent change log rows from the database.
// The change events will be monotically increasing events (monotonic in that
// they're always increasing, but not in that they're always sequential).
// The change events will be ordered by the row id and will be coalesced
// into a single change event if they are for the same entity and for the
// change type.
func (s *Stream) Changes() <-chan changestream.ChangeEvent {
	return s.changes
}

// Kill implements worker.Worker.
func (s *Stream) Kill() {
	s.tomb.Kill(nil)
}

// Wait implements worker.Worker.
func (s *Stream) Wait() error {
	return s.tomb.Wait()
}

func (s *Stream) loop() error {
	// TODO (stickupkid): We need to read the last id from the database and
	// set it here.

	stmt, err := s.db.Prepare(query)
	if err != nil {
		return errors.Annotate(err, "preparing query")
	}
	defer stmt.Close()

	timer := s.clock.NewTimer(PollInterval)
	defer timer.Stop()

	for {
		select {
		case <-s.tomb.Dying():
			return tomb.ErrDying
		case <-timer.Chan():
			changes, err := s.readChanges(stmt)
			if err != nil {
				if errors.Is(err, errRetryable) {
					// We're retrying, so reset the timer to half the poll time, to
					// try and get the changes sooner.
					timer.Reset(PollInterval / 2)
					continue
				}
				return errors.Annotate(err, "reading change")
			}

			for _, change := range changes {
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
SELECT MAX(id), entity_type_id, namespace_id, change_uuid, MAX(created_at)
	FROM change_log WHERE id > ?
	GROUP BY entity_type_id, namespace_id, change_uuid 
	ORDER BY id ASC
`
)

type changeEvent struct {
	id int64
}

// Type returns the type of change (create, update, delete).
func (changeEvent) Type() changestream.ChangeType {
	return changestream.ChangeType(0)
}

// Namespace returns the namespace of the change. This is normally the
// table name.
func (changeEvent) Namespace() string {
	return ""
}

// EntityUUID returns the entity UUID of the change.
func (changeEvent) EntityUUID() string {
	return ""
}

var errRetryable = errors.New("retryable error")

func (w *Stream) readChanges(stmt *sql.Stmt) ([]changeEvent, error) {
	rows, err := stmt.Query(w.lastID)
	if err != nil {
		if database.IsErrRetryable(err) {
			w.logger.Debugf("ignoring error during reading changes: %s", err.Error())
			return nil, errRetryable
		}
		return nil, errors.Annotate(err, "querying for changes")
	}
	defer rows.Close()

	var changes []changeEvent
	dest := func(i int) []interface{} {
		changes = append(changes, changeEvent{})
		return []interface{}{
			&changes[i].id,
		}
	}
	for i := 0; rows.Next(); i++ {
		if err := rows.Scan(dest(i)...); err != nil {
			return nil, errors.Annotate(err, "scanning change")
		}
	}

	return nil, nil
}
