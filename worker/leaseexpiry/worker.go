// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry

import (
	"database/sql"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	coredb "github.com/juju/juju/core/db"
	"github.com/juju/juju/database"
)

// Config encapsulates the configuration options for
// instantiating a new lease expiry worker.
type Config struct {
	Clock     clock.Clock
	Logger    Logger
	TrackedDB coredb.TrackedDB
}

// Validate checks whether the worker configuration settings are valid.
func (cfg Config) Validate() error {
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.TrackedDB == nil {
		return errors.NotValidf("nil TrackedDB")
	}

	return nil
}

type expiryWorker struct {
	catacomb catacomb.Catacomb

	clock     clock.Clock
	logger    Logger
	trackedDB coredb.TrackedDB

	stmt *sql.Stmt
}

// NewWorker returns a worker that periodically deletes
// expired leases from the controller database.
func NewWorker(cfg Config) (worker.Worker, error) {
	var err error

	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &expiryWorker{
		clock:     cfg.Clock,
		logger:    cfg.Logger,
		trackedDB: cfg.TrackedDB,
	}

	// Prepare our single DML statement before considering the worker started.
	// Pinned leases may not be deleted even if expired.
	q := `
DELETE FROM lease WHERE uuid in (
    SELECT l.uuid 
    FROM   lease l LEFT JOIN lease_pin p ON l.uuid = p.lease_uuid
    WHERE  p.uuid IS NULL
    AND    l.expiry < datetime('now')
)`[1:]

	if err := w.trackedDB.Err(); err != nil {
		return nil, errors.Trace(err)
	}

	if w.stmt, err = w.trackedDB.DB().Prepare(q); err != nil {
		return nil, errors.Trace(err)
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *expiryWorker) loop() error {
	timer := w.clock.NewTimer(time.Second)

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-timer.Chan():
			if err := w.expireLeases(); err != nil {
				return errors.Trace(err)
			}
			timer.Reset(time.Second)
		}
	}
}

func (w *expiryWorker) expireLeases() error {
	res, err := w.stmt.Exec()
	if err != nil {
		// TODO (manadart 2022-12-15): This incarnation of the worker runs on
		// all controller nodes. Retryable errors are those that occur due to
		// locking or other contention. We know we will retry very soon,
		// so just log and indicate success for these cases.
		// Rethink this if the worker cardinality changes to be singular.
		if database.IsErrRetryable(err) {
			w.logger.Debugf("ignoring error during lease expiry: %s", err.Error())
			return nil
		}
		return errors.Trace(err)
	}

	expired, err := res.RowsAffected()
	if err != nil {
		return errors.Trace(err)
	}

	if expired > 0 {
		w.logger.Infof("expired %d leases", expired)
	}

	return nil
}

// Kill is part of the worker.Worker interface.
func (w *expiryWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *expiryWorker) Wait() error {
	return w.catacomb.Wait()
}
