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
	coremodel "github.com/juju/juju/core/model"
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

// GetModel is responsible for returning the model with the provided uuid.
func (s *State) GetModel(ctx context.Context, uuid coremodel.UUID) (*coremodel.Model, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT m.model_uuid, m.name, t.type
FROM model_metadata m
INNER JOIN model_type t ON m.model_type_id = t.id
WHERE m.model_uuid = ?`[1:]
	m := coremodel.Model{}
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, q, uuid).Scan(&m.UUID, &m.Name, &m.ModelType)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w model %q%w", errors.NotFound, uuid, errors.Hide(err))
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !m.ModelType.IsValid() {
		// This should never happen.
		return nil, fmt.Errorf("invalid model type for model %q", uuid)
	}
	return &m, nil
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
			return fmt.Errorf("cannot insert secret backend %q: %w", backend.Name, err)
		}
		if backend.NextRotateTime != nil && !backend.NextRotateTime.IsZero() {
			rotateQ := `
INSERT INTO secret_backend_rotation (backend_uuid, next_rotation_time) VALUES
	(?, ?)`[1:]
			_, err = tx.ExecContext(
				ctx, rotateQ, backend.ID,
				sql.NullTime{Time: *backend.NextRotateTime, Valid: true},
			)
			if err != nil {
				return fmt.Errorf("cannot insert secret backend rotation for %q: %w", backend.Name, err)
			}
		}
		configQ := `
INSERT INTO secret_backend_config (backend_uuid, name, content) VALUES
	(?, ?, ?)`[1:]
		for k, v := range backend.Config {
			_, err = tx.ExecContext(ctx, configQ, backend.ID, k, v)
			if err != nil {
				return fmt.Errorf("cannot insert secret backend config for %q: %w", backend.Name, err)
			}
		}
		return err
	})
	return backend.ID, domain.CoerceError(err)
}

// UpdateSecretBackend updates an existing secret backend.
func (s *State) UpdateSecretBackend(ctx context.Context, backend secretbackend.UpdateSecretBackendParams) error {
	if backend.ID == "" {
		return errors.NewNotValid(nil, "backend ID is missing")
	}
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
				return fmt.Errorf(
					"cannot update secret backend name to %q for %q: %w",
					*backend.NameChange, backend.ID, err,
				)
			}
		}
		if backend.TokenRotateInterval != nil {
			rotateQ := `
UPDATE secret_backend SET token_rotate_interval = ? WHERE uuid = ?`[1:]
			_, err := tx.ExecContext(ctx, rotateQ, *backend.TokenRotateInterval, backend.ID)
			if err != nil {
				return fmt.Errorf("cannot update secret backend token rotate interval for %q: %w", backend.ID, err)
			}
		}
		if backend.NextRotateTime != nil {
			rotateQ := `
UPDATE secret_backend_rotation SET next_rotation_time = ? WHERE backend_uuid = ?`[1:]
			_, err := tx.ExecContext(
				ctx, rotateQ,
				sql.NullTime{Time: *backend.NextRotateTime, Valid: true},
				backend.ID,
			)
			if err != nil {
				return fmt.Errorf("cannot update secret backend rotation time for %q: %w", backend.ID, err)
			}
		}
		configQ := `
INSERT INTO secret_backend_config (backend_uuid, name, content) VALUES
	(?, ?, ?)
ON CONFLICT (backend_uuid, name) DO UPDATE SET content = ?`[1:]
		for k, v := range backend.Config {
			_, err = tx.ExecContext(ctx, configQ, backend.ID, k, v, v)
			if err != nil {
				return fmt.Errorf("cannot update secret backend config for %q: %w", backend.ID, err)
			}
		}
		return nil
	})
	return domain.CoerceError(err)
}

// DeleteSecretBackend deletes the secret backend for the given backend ID.
func (s *State) DeleteSecretBackend(ctx context.Context, backendID string, force bool) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}
	// TODO: check if the backend is in use
	// if !force {
	// }
	qCfg := `DELETE FROM secret_backend_config WHERE backend_uuid = ?`
	qRotation := `DELETE FROM secret_backend_rotation WHERE backend_uuid = ?`
	qBackend := `DELETE FROM secret_backend WHERE uuid = ?`
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, qCfg, backendID); err != nil {
			return fmt.Errorf("deleting secret backend config for %q: %w", backendID, err)
		}
		if _, err := tx.ExecContext(ctx, qRotation, backendID); err != nil {
			return fmt.Errorf("deleting secret backend rotation for %q: %w", backendID, err)
		}
		_, err = tx.ExecContext(ctx, qBackend, backendID)
		if err != nil {
			return fmt.Errorf("deleting secret backend for %q: %w", backendID, err)
		}
		return nil
	})
	return domain.CoerceError(err)
}

// ListSecretBackends returns a list of all secret backends.
func (s *State) ListSecretBackends(ctx context.Context) (backends []*secretbackend.SecretBackendInfo, _ error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	q := `
SELECT b.uuid, b.name, b.backend_type, b.token_rotate_interval, c.name, c.content
FROM secret_backend b
LEFT JOIN secret_backend_config c ON b.uuid = c.backend_uuid
ORDER BY b.uuid`[1:]

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q)
		if err != nil {
			return fmt.Errorf("querying secret backends: %w", err)
		}
		defer rows.Close()

		var (
			lastID         string
			currentBackend *secretbackend.SecretBackendInfo
		)
		for rows.Next() {
			var (
				backend             secretbackend.SecretBackendInfo
				tokenRotateInterval NullableDuration
				name, content       sql.NullString
			)
			if err := rows.Scan(
				&backend.ID, &backend.Name, &backend.BackendType, &tokenRotateInterval,
				&name, &content,
			); err != nil {
				return fmt.Errorf("scanning secret backend: %w", err)
			}
			if tokenRotateInterval.Valid {
				backend.TokenRotateInterval = &tokenRotateInterval.Duration
			}

			if currentBackend == nil || backend.ID != lastID {
				// Encountered a new backend.
				currentBackend = &backend
				lastID = backend.ID
				backends = append(backends, currentBackend)
			}

			if !name.Valid || !content.Valid {
				continue
			}
			if currentBackend.Config == nil {
				currentBackend.Config = make(map[string]interface{})
			}
			currentBackend.Config[name.String] = content.String
		}
		if err = rows.Err(); err != nil {
			return fmt.Errorf("scanning secret backends: %w", err)
		}
		return nil
	})
	return backends, errors.Trace(err)
}

func (s *State) getSecretBackend(ctx context.Context, k string, v string) (*coresecrets.SecretBackend, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	q := fmt.Sprintf(`
SELECT b.uuid, b.name, b.backend_type, b.token_rotate_interval, c.name, c.content
FROM secret_backend b
LEFT JOIN secret_backend_config c ON b.uuid = c.backend_uuid
WHERE b.%s = ?`, k)[1:]
	var backend coresecrets.SecretBackend
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) (err error) {
		rows, err := tx.QueryContext(ctx, q, v)
		if err != nil {
			return fmt.Errorf("querying secret backend: %w", err)
		}
		defer rows.Close()
		defer func() {
			if err != nil {
				err = fmt.Errorf("cannot get secret backend %q:%w", v, err)
			} else if backend.ID == "" {
				err = errors.NotFoundf("secret backend %q", v)
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
				return fmt.Errorf("scanning secret backend: %w", err)
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
	return s.getSecretBackend(ctx, "name", backendName)
}

// GetSecretBackend returns the secret backend for the given backend ID.
func (s *State) GetSecretBackend(ctx context.Context, backendID string) (*coresecrets.SecretBackend, error) {
	return s.getSecretBackend(ctx, "uuid", backendID)
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
	q := `
UPDATE secret_backend_rotation
SET next_rotation_time = ?
WHERE backend_uuid = ? AND next_rotation_time > ?`[1:]
	nextN := sql.NullTime{
		Time:  next,
		Valid: true,
	}
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, nextN, backendID, nextN)
		if err != nil {
			return fmt.Errorf("updating secret backend rotation: %w", err)
		}
		return nil
	})
	return domain.CoerceError(err)
}

// WatchSecretBackendRotationChanges returns a watcher for secret backend rotation changes.
func (s *State) WatchSecretBackendRotationChanges(wf secretbackend.WatcherFactory) (secretbackend.SecretBackendRotateWatcher, error) {
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
	w.logger.Debugf("processing secret backend rotation changes for: %v", backendIDs)

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
			return fmt.Errorf("querying secret backend rotation changes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var change watcher.SecretBackendRotateChange
			var next sql.NullTime

			err = rows.Scan(&change.ID, &change.Name, &next)
			if err != nil {
				return fmt.Errorf("scanning secret backend rotation changes: %w", err)
			}
			if !next.Valid {
				w.logger.Warningf("secret backend %q has no next rotation time", change.ID)
				continue
			}
			change.NextTriggerTime = next.Time

			w.logger.Debugf(
				"backend rotation change processed: %q, %q, %q",
				change.ID, change.Name, change.NextTriggerTime,
			)
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
