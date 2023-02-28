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
	"github.com/juju/worker/v3/catacomb"

	coredb "github.com/juju/juju/core/db"
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
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Prepare our single DML statement before considering the worker started.
	// Pinned leases may not be deleted even if expired.
	q := `
DELETE FROM lease WHERE uuid in (
    SELECT l.uuid 
    FROM   lease l LEFT JOIN lease_pin p ON l.uuid = p.lease_uuid
    WHERE  p.uuid IS NULL
    AND    l.expiry < datetime('now')
)`[1:]

	err := w.trackedDB.Txn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, q)
		if err != nil {
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
	return errors.Trace(err)
}

// Kill is part of the worker.Worker interface.
func (w *expiryWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *expiryWorker) Wait() error {
	return w.catacomb.Wait()
}
