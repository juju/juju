// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/database"
)

// NullableDuration represents a nullable time.Duration.
type NullableDuration struct {
	Duration time.Duration
	Valid    bool
}

// Scan implements the sql.Scanner interface.
func (nd *NullableDuration) Scan(value interface{}) error {
	if value == nil {
		nd.Duration, nd.Valid = 0, false
		return nil
	}
	switch v := value.(type) {
	case int64:
		nd.Duration = time.Duration(v)
		nd.Valid = true
	default:
		return fmt.Errorf("cannot scan type %T into NullableDuration", value)
	}
	return nil
}

// Value implements the driver.Valuer interface.
func (nd NullableDuration) Value() (driver.Value, error) {
	if !nd.Valid {
		return nil, nil
	}
	return int64(nd.Duration), nil
}

func dateTimeToString(t time.Time) string {
	return t.UTC().Round(time.Second).Format(time.RFC3339)
}

func stringToDatetime(s string) (time.Time, error) {
	dt, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return dt.Round(time.Second), nil
}

// Logger is the interface used by the state to log messages.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
}

// State represents database interactions dealing with secret backends.
type State struct {
	*domain.StateBase
	logger Logger
}

// NewState returns a new secret backend state based on the input database factory method.
func NewState(factory coredatabase.TxnRunnerFactory, logger Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// CreateSecretBackend adds a new secret backend.
func (s *State) CreateSecretBackend(ctx context.Context, backend secretbackend.CreateSecretBackendParams) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		backendQ := `
INSERT INTO secret_backend (uuid, name, backend_type, token_rotate_interval) VALUES
	(?, ?, ?, ?)`[1:]
		_, err := tx.ExecContext(ctx, backendQ,
			backend.ID, backend.Name, backend.BackendType, backend.TokenRotateInterval,
		)
		if err != nil {
			return errors.Annotatef(err, "cannot insert secret backend %q", backend.Name)
		}
		if backend.NextRotateTime != nil && !backend.NextRotateTime.IsZero() {
			rotateQ := `
INSERT INTO secret_backend_rotation (backend_uuid, next_rotation_time) VALUES
	(?, ?)`[1:]
			_, err = tx.ExecContext(
				ctx, rotateQ, backend.ID,
				dateTimeToString(*backend.NextRotateTime),
			)
			if err != nil {
				return errors.Annotatef(err, "cannot insert secret backend rotation for %q", backend.Name)
			}
		}
		configQ := `
INSERT INTO secret_backend_config (backend_uuid, name, content) VALUES
	(?, ?, ?)`[1:]
		for k, v := range backend.Config {
			_, err = tx.ExecContext(ctx, configQ, backend.ID, k, v)
			if err != nil {
				return errors.Annotatef(err, "cannot insert secret backend config for %q", backend.Name)
			}
		}
		return err
	})
	return backend.ID, errors.Trace(err)
}

// UpdateSecretBackend updates an existing secret backend.
func (s *State) UpdateSecretBackend(ctx context.Context, backend secretbackend.UpdateSecretBackendParams) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if backend.NameChange != nil {
			nameQ := `
UPDATE secret_backend SET name = ? WHERE uuid = ?`[1:]
			_, err := tx.ExecContext(ctx, nameQ, *backend.NameChange, backend.ID)
			if err != nil {
				return errors.Annotatef(err, "cannot update secret backend name for %q", backend.ID)
			}
		}
		if backend.TokenRotateInterval != nil {
			rotateQ := `
UPDATE secret_backend SET token_rotate_interval = ? WHERE uuid = ?`[1:]
			_, err := tx.ExecContext(ctx, rotateQ, *backend.TokenRotateInterval, backend.ID)
			if err != nil {
				return errors.Annotatef(err, "cannot update secret backend rotation for %q", backend.ID)
			}
		}
		if backend.NextRotateTime != nil {
			rotateQ := `
UPDATE secret_backend_rotation SET next_rotation_time = ? WHERE backend_uuid = ?`[1:]
			_, err := tx.ExecContext(
				ctx, rotateQ,
				dateTimeToString(*backend.NextRotateTime),
				backend.ID,
			)
			if err != nil {
				return errors.Annotatef(err, "cannot update secret backend rotation for %q", backend.ID)
			}
		}
		configQ := `
INSERT INTO secret_backend_config (backend_uuid, name, content) VALUES
	(?, ?, ?)
ON CONFLICT (backend_uuid, name) DO UPDATE SET content = ?`[1:]
		for k, v := range backend.Config {
			_, err = tx.ExecContext(ctx, configQ, backend.ID, k, v, v)
			if err != nil {
				return errors.Annotatef(err, "cannot update secret backend config for %q", backend.ID)
			}
		}
		return err
	})
	return errors.Trace(err)
}

// DeleteSecretBackend deletes the secret backend for the given backend ID.
func (s *State) DeleteSecretBackend(ctx context.Context, backendID string, force bool) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}
	// TODO: How to track if the secret backend is `in-use` for `Force`!!!
	// secret_backend.secret_count++ whenever a secret is created?
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM secret_backend WHERE uuid = ?", backendID)
		return err
	})
	return errors.Trace(err)
}

// ListSecretBackends returns a list of all secret backends.
func (s *State) ListSecretBackends(ctx context.Context) ([]*coresecrets.SecretBackend, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var backends []*coresecrets.SecretBackend
	// TODO: use Prepare() query for better performance.
	q := `
SELECT uuid, name, backend_type, token_rotate_interval
FROM secret_backend`[1:]
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			backend := coresecrets.SecretBackend{
				Config: make(map[string]interface{}),
			}
			var tokenRotateInterval NullableDuration
			err = rows.Scan(
				&backend.ID, &backend.Name, &backend.BackendType, &tokenRotateInterval,
			)
			if err != nil {
				return err
			}
			if tokenRotateInterval.Valid {
				backend.TokenRotateInterval = &tokenRotateInterval.Duration
			}
			configQ := `
SELECT name, content
FROM secret_backend_config
WHERE backend_uuid = ?`[1:]
			configRows, err := tx.QueryContext(ctx, configQ, backend.ID)
			if err != nil {
				return err
			}
			defer configRows.Close()
			for configRows.Next() {
				var name, content string
				err = configRows.Scan(&name, &content)
				if err != nil {
					return err
				}
				backend.Config[name] = content
			}
			backends = append(backends, &backend)
		}
		return rows.Err()
	})
	return backends, errors.Trace(err)
}

func (s *State) getSecretBackend(
	ctx context.Context, query func(context.Context, *sql.Tx) (*sql.Rows, string, error),
) (*coresecrets.SecretBackend, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var backend coresecrets.SecretBackend
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) (err error) {
		rows, identifier, err := query(ctx, tx)
		if err != nil {
			return err
		}
		defer rows.Close()
		defer func() {
			if err != nil {
				err = errors.Annotatef(err, "cannot get secret backend %q", identifier)
			} else if backend.ID == "" {
				err = errors.NotFoundf("secret backend %q", identifier)
			}
		}()
		for rows.Next() {
			var name, content sql.NullString
			var tokenRotateInterval NullableDuration
			err = rows.Scan(
				&backend.ID, &backend.Name, &backend.BackendType, &tokenRotateInterval,
				&name, &content,
			)
			if err != nil {
				return err
			}
			if tokenRotateInterval.Valid {
				backend.TokenRotateInterval = &tokenRotateInterval.Duration
			}
			if !name.Valid || !content.Valid {
				continue
			}
			if backend.Config == nil {
				backend.Config = make(map[string]interface{})
			}
			backend.Config[name.String] = content.String
		}
		return rows.Err()
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &backend, nil
}

// GetSecretBackendByName returns the secret backend for the given backend name.
func (s *State) GetSecretBackendByName(ctx context.Context, backendName string) (*coresecrets.SecretBackend, error) {
	q := `
SELECT b.uuid, b.name, b.backend_type, b.token_rotate_interval, c.name, c.content
FROM secret_backend b
LEFT JOIN secret_backend_config c ON b.uuid = c.backend_uuid
WHERE b.name = ?`[1:]
	return s.getSecretBackend(ctx, func(ctx context.Context, tx *sql.Tx) (*sql.Rows, string, error) {
		rows, err := tx.QueryContext(ctx, q, backendName)
		return rows, backendName, err
	})
}

// GetSecretBackend returns the secret backend for the given backend ID.
func (s *State) GetSecretBackend(ctx context.Context, backendID string) (*coresecrets.SecretBackend, error) {
	q := `
SELECT b.uuid, b.name, b.backend_type, b.token_rotate_interval, c.name, c.content
FROM secret_backend b
LEFT JOIN secret_backend_config c ON b.uuid = c.backend_uuid
WHERE b.uuid = ?`[1:]
	return s.getSecretBackend(ctx, func(ctx context.Context, tx *sql.Tx) (*sql.Rows, string, error) {
		rows, err := tx.QueryContext(ctx, q, backendID)
		return rows, backendID, err
	})
}

// SecretBackendRotated updates the next rotation time for the secret backend.
func (s *State) SecretBackendRotated(ctx context.Context, backendID string, next time.Time) error {
	if _, err := s.GetSecretBackend(ctx, backendID); err != nil {
		// Check if the backend exists or not here because the
		// below UPDATE operation won't tell us.
		return errors.Trace(err)
	}
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}
	nextS := dateTimeToString(next)
	q := `
UPDATE secret_backend_rotation
SET next_rotation_time = ?
WHERE backend_uuid = ? AND next_rotation_time > ?`[1:]
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, nextS, backendID, nextS)
		return err
	})
	return errors.Trace(err)
}

// WatchSecretBackendRotationChanges returns a watcher for secret backend rotation changes.
func (s *State) WatchSecretBackendRotationChanges(ctx context.Context, wf secretbackend.WatcherFactory) (watcher.SecretBackendRotateWatcher, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	initialQ := `SELECT backend_uuid FROM secret_backend_rotate`
	w, err := wf.NewNamespaceWatcher("secret_backend_rotate", changestream.All, initialQ)
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
	defer func() {
		if err != nil {
			w.logger.Warningf("secret backend rotation watcher stopped, err: %v", err)
		}
	}()
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
			w.logger.Debugf("processing secret backend rotation changes: %v", backendIDs)
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

func (w *secretBackendRotateWatcher) processChanges(ctx context.Context, backendIDs ...string) ([]watcher.SecretBackendRotateChange, error) {
	placeholders, values := database.SliceToPlaceholder(backendIDs)
	q := fmt.Sprintf(`
SELECT b.uuid, b.name, r.next_rotation_time
FROM secret_backend b
LEFT JOIN secret_backend_rotation r ON b.uuid = r.backend_uuid
WHERE b.uuid IN (%s)`[1:], placeholders)

	var changes []watcher.SecretBackendRotateChange
	err := w.db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q, values...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var change watcher.SecretBackendRotateChange
			var next sql.NullString
			err = rows.Scan(&change.ID, &change.Name, &next)
			if err != nil {
				return err
			}
			if !next.Valid {
				w.logger.Warningf("secret backend %q has no next rotation time", change.ID)
				continue
			}
			w.logger.Debugf("backend rotation change: %q, %q, %q", change.ID, change.Name, next.String)
			if change.NextTriggerTime, err = stringToDatetime(next.String); err != nil {
				return errors.Trace(err)
			}
			changes = append(changes, change)
		}
		return rows.Err()
	})
	return changes, errors.Trace(err)
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
