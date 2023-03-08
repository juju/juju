// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/database"
)

// Config encapsulates the configuration options for
// instantiating a new lease expiry worker.
type Config struct {
	Clock     clock.Clock
	Logger    Logger
	TrackedDB coredatabase.TrackedDB
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
	trackedDB coredatabase.TrackedDB

	mutex sync.Mutex
	stmt  *sql.Stmt
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

	if err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *expiryWorker) loop() error {
	// Now that we have a statement, we can start the worker.
	timer := w.clock.NewTimer(time.Second)
	defer timer.Stop()

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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := w.trackedDB.DB(func(db *sql.DB) error {
		// Txn here provides the transaction semantics we need, but there are
		// no retry strategies employed. This is a special case, in this instance
		// we will retry this in one second and other controllers in a HA
		// setup will also retry.
		return database.Txn(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
			q := `
		DELETE FROM lease WHERE uuid in (
			SELECT l.uuid 
			FROM   lease l LEFT JOIN lease_pin p ON l.uuid = p.lease_uuid
			WHERE  p.uuid IS NULL
			AND    l.expiry < datetime('now')
		)`[1:]

			res, err := tx.ExecContext(ctx, q)
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
		})
	})
	if err != nil {
		if database.IsErrRetryable(err) {
			return nil
		}
		return errors.Trace(err)
	}
	if w.trackedDB.Err() != nil {
		return errors.Trace(w.trackedDB.Err())
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
