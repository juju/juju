// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/secretbackend"
)

// WatchSecretBackendRotationChanges returns a watcher for secret backend rotation changes.
func (s *State) WatchSecretBackendRotationChanges(wf secretbackend.WatcherFactory) (watcher.SecretBackendRotateWatcher, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	initialQ := `SELECT backend_uuid FROM secret_backend_rotation`
	w, err := wf.NewNamespaceWatcher("secret_backend_rotation", changestream.All, initialQ)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newSecretBackendRotateWatcher(w, db, s.logger), nil
}

type secretBackendRotateWatcher struct {
	tomb          tomb.Tomb
	sourceWatcher watcher.StringsWatcher
	db            coredatabase.TxnRunner
	logger        Logger

	out chan []watcher.SecretBackendRotateChange

	backendFetchStatement *sqlair.Statement
}

func newSecretBackendRotateWatcher(
	sourceWatcher watcher.StringsWatcher, db coredatabase.TxnRunner, logger Logger,
) *secretBackendRotateWatcher {
	w := &secretBackendRotateWatcher{
		sourceWatcher: sourceWatcher,
		db:            db,
		logger:        logger,
		out:           make(chan []watcher.SecretBackendRotateChange),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

func (w *secretBackendRotateWatcher) loop() (err error) {
	if err = w.prepareBackendFetchStatement(); err != nil {
		return errors.Trace(err)
	}

	// To allow the initial event to sent.
	out := w.out
	var changes []watcher.SecretBackendRotateChange
	ctx := w.tomb.Context(context.Background())
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case backendIDs, ok := <-w.sourceWatcher.Changes():
			if !ok {
				return errors.Errorf("event watcher closed")
			}
			w.logger.Debugf("received secret backend rotation changes: %v", backendIDs)

			var err error
			changes, err = w.processChanges(ctx, backendIDs...)
			if err != nil {
				return errors.Trace(err)
			}
			if len(changes) > 0 {
				out = w.out
			}
		case out <- changes:
			out = nil
		}
	}
}

func (w *secretBackendRotateWatcher) prepareBackendFetchStatement() (err error) {
	w.backendFetchStatement, err = sqlair.Prepare(`
SELECT 
    b.uuid               AS &SecretBackendRotationRow.uuid,
    b.name               AS &SecretBackendRotationRow.name,
    r.next_rotation_time AS &SecretBackendRotationRow.next_rotation_time
FROM secret_backend b
    LEFT JOIN secret_backend_rotation r ON b.uuid = r.backend_uuid
WHERE b.uuid IN ($S[:])`,
		sqlair.S{}, SecretBackendRotationRow{},
	)
	return errors.Trace(err)
}

func (w *secretBackendRotateWatcher) processChanges(ctx context.Context, backendIDs ...string) ([]watcher.SecretBackendRotateChange, error) {
	var rows SecretBackendRotationRows
	args := sqlair.S(transform.Slice(backendIDs, func(s string) any { return any(s) }))
	err := w.db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, w.backendFetchStatement, args).GetAll(&rows)
		if err != nil {
			return fmt.Errorf("querying secret backend rotation changes: %w", err)
		}
		return nil
	})
	return rows.ToChanges(w.logger), errors.Trace(err)
}

// Changes returns the channel of secret backend rotation changes.
func (w *secretBackendRotateWatcher) Changes() <-chan []watcher.SecretBackendRotateChange {
	return w.out
}

// Kill (worker.Worker) kills the watcher via its tomb.
func (w *secretBackendRotateWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait (worker.Worker) waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *secretBackendRotateWatcher) Wait() error {
	return w.tomb.Wait()
}
