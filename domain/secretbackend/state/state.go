// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
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

	stmt, err := sqlair.Prepare(`
SELECT 
    m.model_uuid AS &Model.uuid,
    m.name       AS &Model.name,
    t.type       AS &Model.type
FROM model_metadata m
INNER JOIN model_type t ON m.model_type_id = t.id
WHERE m.model_uuid = $M.uuid`, sqlair.M{}, Model{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	var m Model
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, sqlair.M{"uuid": uuid}).Get(&m)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w model %q%w", errors.NotFound, uuid, errors.Hide(err))
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !m.Type.IsValid() {
		// This should never happen.
		return nil, fmt.Errorf("invalid model type for model %q", uuid)
	}
	return &coremodel.Model{
		UUID:      coremodel.UUID(m.UUID),
		Name:      m.Name,
		ModelType: m.Type,
	}, nil
}

// CreateSecretBackend adds a new secret backend.
func (s *State) CreateSecretBackend(ctx context.Context, backend secretbackend.CreateSecretBackendParams) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	backendStmt, err := sqlair.Prepare(`
INSERT INTO secret_backend
(uuid, name, backend_type, token_rotate_interval) VALUES
(
    $SecretBackend.uuid,
    $SecretBackend.name,
    $SecretBackend.backend_type,
    $SecretBackend.token_rotate_interval
)`, SecretBackend{})
	if err != nil {
		return "", errors.Trace(err)
	}
	rotateStmt, err := sqlair.Prepare(`
INSERT INTO secret_backend_rotation
(backend_uuid, next_rotation_time) VALUES
(
    $SecretBackendRotation.backend_uuid,
    $SecretBackendRotation.next_rotation_time
)`, SecretBackendRotation{})
	if err != nil {
		return "", errors.Trace(err)
	}
	configStmt, err := sqlair.Prepare(`
INSERT INTO secret_backend_config
(backend_uuid, name, content) VALUES
(
    $SecretBackendConfig.backend_uuid,
    $SecretBackendConfig.name,
    $SecretBackendConfig.content
)`, SecretBackendConfig{})
	if err != nil {
		return "", errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		sb := SecretBackend{
			UUID:        backend.ID,
			Name:        backend.Name,
			BackendType: backend.BackendType,
		}
		if backend.TokenRotateInterval != nil {
			sb.TokenRotateInterval = domain.NullableDuration{Duration: *backend.TokenRotateInterval, Valid: true}
		}
		err := tx.Query(ctx, backendStmt, sb).Run()
		if err != nil {
			return fmt.Errorf("cannot insert secret backend %q: %w", backend.Name, err)
		}
		if backend.NextRotateTime != nil && !backend.NextRotateTime.IsZero() {
			err = tx.Query(ctx, rotateStmt,
				SecretBackendRotation{
					BackendUUID:      backend.ID,
					NextRotationTime: sql.NullTime{Time: *backend.NextRotateTime, Valid: true},
				},
			).Run()
			if err != nil {
				return fmt.Errorf("cannot insert secret backend rotation for %q: %w", backend.Name, err)
			}
		}
		for k, v := range backend.Config {
			err = tx.Query(ctx, configStmt,
				SecretBackendConfig{
					BackendUUID: backend.ID,
					Name:        k,
					Content:     v.(string),
				},
			).Run()
			if err != nil {
				return handleSecretBackendConfigError(err, backend.Name)
			}
		}
		return err
	})
	return backend.ID, domain.CoerceError(err)
}

func handleSecretBackendConfigError(err error, identifier string) error {
	if err == nil {
		return nil
	}
	if database.IsErrConstraintCheck(err) {
		return fmt.Errorf(
			"cannot upsert secret backend config for %q: %w",
			identifier, errors.NewNotValid(nil, "empty config name or content"),
		)
	}
	return fmt.Errorf("cannot upsert secret backend config for %q: %w", identifier, err)
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
	nameStmt, err := sqlair.Prepare(`
UPDATE secret_backend
SET name = $SecretBackend.name
WHERE uuid = $SecretBackend.uuid`, SecretBackend{})
	if err != nil {
		return errors.Trace(err)
	}
	rotateIntervalStmt, err := sqlair.Prepare(`
UPDATE secret_backend
SET token_rotate_interval = $SecretBackend.token_rotate_interval
WHERE uuid = $SecretBackend.uuid`, SecretBackend{})
	if err != nil {
		return errors.Trace(err)
	}
	rotationTimeStmt, err := sqlair.Prepare(`
UPDATE secret_backend_rotation
SET next_rotation_time = $SecretBackendRotation.next_rotation_time
WHERE backend_uuid = $SecretBackendRotation.backend_uuid`, SecretBackendRotation{})
	if err != nil {
		return errors.Trace(err)
	}
	configStmt, err := sqlair.Prepare(`
INSERT INTO secret_backend_config (backend_uuid, name, content) VALUES
(
    $SecretBackendConfig.backend_uuid,
    $SecretBackendConfig.name,
    $SecretBackendConfig.content
)
ON CONFLICT (backend_uuid, name) DO UPDATE SET content = EXCLUDED.content`, SecretBackendConfig{})
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if backend.NameChange != nil {
			err := tx.Query(ctx, nameStmt,
				SecretBackend{
					UUID: backend.ID,
					Name: *backend.NameChange,
				},
			).Run()
			if database.IsErrConstraintUnique(err) {
				return fmt.Errorf("secret backend name %q: %w", *backend.NameChange, errors.AlreadyExists)
			}
			if err != nil {
				return fmt.Errorf(
					"cannot update secret backend name to %q for %q: %w",
					*backend.NameChange, backend.ID, err,
				)
			}
		}
		if backend.TokenRotateInterval != nil {
			err := tx.Query(ctx, rotateIntervalStmt,
				SecretBackend{
					UUID:                backend.ID,
					TokenRotateInterval: domain.NullableDuration{Duration: *backend.TokenRotateInterval, Valid: true},
				},
			).Run()
			if err != nil {
				return fmt.Errorf("cannot update secret backend token rotate interval for %q: %w", backend.ID, err)
			}
		}
		if backend.NextRotateTime != nil {
			err := tx.Query(ctx, rotationTimeStmt,
				SecretBackendRotation{
					BackendUUID:      backend.ID,
					NextRotationTime: sql.NullTime{Time: *backend.NextRotateTime, Valid: true},
				},
			).Run()
			if err != nil {
				return fmt.Errorf("cannot update secret backend rotation time for %q: %w", backend.ID, err)
			}
		}
		for k, v := range backend.Config {
			err = tx.Query(ctx, configStmt,
				SecretBackendConfig{
					BackendUUID: backend.ID,
					Name:        k,
					Content:     v.(string),
				},
			).Run()
			if err != nil {
				return handleSecretBackendConfigError(err, backend.ID)
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
	cfgStmt, err := sqlair.Prepare(`
DELETE FROM secret_backend_config WHERE backend_uuid = $M.uuid`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	rotationStmt, err := sqlair.Prepare(`
DELETE FROM secret_backend_rotation WHERE backend_uuid = $M.uuid`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	// TODO: we should set it to the `default` backend once we start to include the
	// `internal` and `k8s` backends in the database.
	// For now, we reset it to NULL.
	modelMetadataStmt, err := sqlair.Prepare(`
UPDATE model_metadata
SET secret_backend_uuid = NULL
WHERE secret_backend_uuid = $M.uuid`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	backendStmt, err := sqlair.Prepare(`
DELETE FROM secret_backend WHERE uuid = $M.uuid`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		arg := sqlair.M{"uuid": backendID}
		if err := tx.Query(ctx, cfgStmt, arg).Run(); err != nil {
			return fmt.Errorf("deleting secret backend config for %q: %w", backendID, err)
		}
		if err := tx.Query(ctx, rotationStmt, arg).Run(); err != nil {
			return fmt.Errorf("deleting secret backend rotation for %q: %w", backendID, err)
		}
		if err = tx.Query(ctx, modelMetadataStmt, arg).Run(); err != nil {
			return fmt.Errorf("resetting secret backend %q to NULL for model metadata: %w", backendID, err)
		}
		if err = tx.Query(ctx, backendStmt, arg).Run(); err != nil {
			return fmt.Errorf("deleting secret backend for %q: %w", backendID, err)
		}
		return nil
	})
	return domain.CoerceError(err)
}

// ListSecretBackends returns a list of all secret backends.
func (s *State) ListSecretBackends(ctx context.Context) ([]*secretbackend.SecretBackendInfo, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	stmt, err := sqlair.Prepare(`
SELECT 
    b.uuid                  AS &SecretBackendRow.uuid,
    b.name                  AS &SecretBackendRow.name,
    b.backend_type          AS &SecretBackendRow.backend_type,
    b.token_rotate_interval AS &SecretBackendRow.token_rotate_interval,
    c.name                  AS &SecretBackendRow.config_name,
    c.content               AS &SecretBackendRow.config_content
FROM secret_backend b
    LEFT JOIN secret_backend_config c ON b.uuid = c.backend_uuid
ORDER BY b.name`, SecretBackendRow{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var rows SecretBackendRows
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if err != nil {
			return fmt.Errorf("querying secret backends: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot list secret backends: %w", err)
	}
	var result []*secretbackend.SecretBackendInfo
	for _, backend := range rows.ToSecretBackendInfo() {
		result = append(result, &secretbackend.SecretBackendInfo{
			SecretBackend: *backend,
		})
	}
	return result, errors.Trace(err)
}

func (s *State) getSecretBackend(ctx context.Context, columName string, v string) (*coresecrets.SecretBackend, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	q := fmt.Sprintf(`
SELECT
    b.uuid                  AS &SecretBackendRow.uuid,
    b.name                  AS &SecretBackendRow.name,
    b.backend_type          AS &SecretBackendRow.backend_type,
    b.token_rotate_interval AS &SecretBackendRow.token_rotate_interval,
    c.name                  AS &SecretBackendRow.config_name,
    c.content               AS &SecretBackendRow.config_content
FROM secret_backend b
    LEFT JOIN secret_backend_config c ON b.uuid = c.backend_uuid
WHERE b.%s = $M.identifier`, columName)
	stmt, err := sqlair.Prepare(q, sqlair.M{}, SecretBackendRow{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var rows SecretBackendRows
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, sqlair.M{"identifier": v}).GetAll(&rows)
		if err != nil {
			return fmt.Errorf("querying secret backends: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot list secret backends: %w", err)
	}
	if len(rows) == 0 {
		return nil, errors.NotFoundf("secret backend %q", v)
	}
	return rows.ToSecretBackendInfo()[0], errors.Trace(err)
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
	stmt, err := sqlair.Prepare(`
UPDATE secret_backend_rotation
SET next_rotation_time = $SecretBackendRotation.next_rotation_time
WHERE backend_uuid = $SecretBackendRotation.backend_uuid
    AND next_rotation_time > $SecretBackendRotation.next_rotation_time`, SecretBackendRotation{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt,
			SecretBackendRotation{
				BackendUUID:      backendID,
				NextRotationTime: sql.NullTime{Time: next, Valid: true},
			},
		).Run()
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

	stmt *sqlair.Statement
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
	if err = w.prepareStatement(); err != nil {
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

func (w *secretBackendRotateWatcher) prepareStatement() (err error) {
	w.stmt, err = sqlair.Prepare(`
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
		err := tx.Query(ctx, w.stmt, args).GetAll(&rows)
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
