// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/database/txn"
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
	tomb tomb.Tomb

	clock     clock.Clock
	logger    Logger
	trackedDB coredatabase.TrackedDB
	dml       string
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
		dml: `
DELETE FROM lease WHERE uuid in (
    SELECT l.uuid 
    FROM   lease l LEFT JOIN lease_pin p ON l.uuid = p.lease_uuid
    WHERE  p.uuid IS NULL
    AND    l.expiry < datetime('now')
)`[1:],
	}

	w.tomb.Go(w.loop)
	return w, nil

}

func (w *expiryWorker) loop() error {
	timer := w.clock.NewTimer(time.Second)
	defer timer.Stop()

	// We pass this context to every database method that accepts one.
	// It is cancelled by killing the tomb, which prevents shutdown
	// being blocked by such calls.
	ctx := w.tomb.Context(context.Background())

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-timer.Chan():
			if err := w.expireLeases(ctx); err != nil {
				return errors.Trace(err)
			}
			timer.Reset(time.Second)
		}
	}
}

func (w *expiryWorker) expireLeases(ctx context.Context) error {
	err := w.trackedDB.TxnNoRetry(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, w.dml)
		if err != nil {
			// TODO (manadart 2022-12-15): This incarnation of the worker runs on
			// all controller nodes. Retryable errors are those that occur due to
			// locking or other contention. We know we will retry very soon,
			// so just log and indicate success for these cases.
			// Rethink this if the worker cardinality changes to be singular.
			if txn.IsErrRetryable(err) {
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
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(w.trackedDB.Err())
}

// Kill is part of the worker.Worker interface.
func (w *expiryWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *expiryWorker) Wait() error {
	return w.tomb.Wait()
}
