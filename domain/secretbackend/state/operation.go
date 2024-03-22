// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/domain/secretbackend"
	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/database"
)

type upsertOperation struct {
	secretbackend.UpsertSecretBackendParams

	getBackendStmt *sqlair.Statement

	upsertBackendStmt                 *sqlair.Statement
	upsertRotationStmt                *sqlair.Statement
	upsertConfigStmt, clearConfigStmt *sqlair.Statement
}

// build builds the sqlair statements.
func (o *upsertOperation) build() (err error) {
	// validate the input parameters here, so we can fail early without accessing the database.
	if o.ID == "" {
		return fmt.Errorf("%w: ID is missing", backenderrors.NotValid)
	}
	for k, v := range o.Config {
		if k == "" {
			return fmt.Errorf(
				"%w: empty config key for %q", backenderrors.NotValid, o.ID)
		}
		if v.(string) == "" {
			return fmt.Errorf(
				"%w: empty config value for %q", backenderrors.NotValid, o.ID)
		}
	}

	o.getBackendStmt, err = sqlair.Prepare(`
SELECT &SecretBackend.*
FROM secret_backend
WHERE uuid = $M.uuid`, SecretBackend{}, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	o.upsertBackendStmt, err = sqlair.Prepare(`
INSERT INTO secret_backend
    (uuid, name, backend_type, token_rotate_interval)
VALUES (
    $SecretBackend.uuid,
    $SecretBackend.name,
    $SecretBackend.backend_type,
    $SecretBackend.token_rotate_interval
)
ON CONFLICT (uuid) DO UPDATE SET
    name=EXCLUDED.name,
    token_rotate_interval=EXCLUDED.token_rotate_interval;`, SecretBackend{})
	if err != nil {
		return errors.Trace(err)
	}

	o.upsertRotationStmt, err = sqlair.Prepare(`
INSERT INTO secret_backend_rotation
    (backend_uuid, next_rotation_time)
VALUES (
    $SecretBackendRotation.backend_uuid,
    $SecretBackendRotation.next_rotation_time
)
ON CONFLICT (backend_uuid) DO UPDATE SET next_rotation_time=EXCLUDED.next_rotation_time;`,
		SecretBackendRotation{})
	if err != nil {
		return errors.Trace(err)
	}

	o.clearConfigStmt, err = sqlair.Prepare(`
DELETE FROM secret_backend_config
WHERE backend_uuid = $M.uuid AND name NOT IN ($S[:]);`,
		sqlair.M{}, sqlair.S{})
	if err != nil {
		return errors.Trace(err)
	}

	o.upsertConfigStmt, err = sqlair.Prepare(`
INSERT INTO secret_backend_config
    (backend_uuid, name, content)
VALUES (
    $SecretBackendConfig.backend_uuid,
    $SecretBackendConfig.name,
    $SecretBackendConfig.content
)
ON CONFLICT (backend_uuid, name) DO UPDATE SET content = EXCLUDED.content`,
		SecretBackendConfig{})
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (o *upsertOperation) prepareData(ctx context.Context, tx *sqlair.TX) error {
	var existing SecretBackend
	err := tx.Query(ctx, o.getBackendStmt, sqlair.M{"uuid": o.ID}).Get(&existing)
	if errors.Is(err, sqlair.ErrNoRows) {
		// New insert.
		if o.Name == "" {
			return fmt.Errorf("%w: name is missing", backenderrors.NotValid)
		}
		if o.BackendType == "" {
			return fmt.Errorf("%w: type is missing", backenderrors.NotValid)
		}
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	// Update.
	if existing.BackendType == "" {
		return errors.Errorf("backend type is empty for backend %q", o.ID)
	}
	if existing.Name == "" {
		return errors.Errorf("backend name is empty for backend %q", o.ID)
	}
	if o.BackendType != "" && o.BackendType != existing.BackendType {
		// The secret backend type is immutable.
		return errors.Errorf(
			"cannot change backend type from %q to %q because backend type is immutable",
			existing.BackendType, o.BackendType,
		)
	}
	// Fill in the existing backend type.
	o.BackendType = existing.BackendType
	if o.Name == "" {
		// Fill in the existing name.
		o.Name = existing.Name
	}
	if o.TokenRotateInterval == nil && existing.TokenRotateInterval.Valid {
		// Fill in the existing token rotate interval.
		o.TokenRotateInterval = &existing.TokenRotateInterval.Duration
	}
	return nil
}

// apply applies the upsert operation to the database.
func (o *upsertOperation) apply(ctx context.Context, tx *sqlair.TX) error {
	if err := o.prepareData(ctx, tx); err != nil {
		return errors.Trace(err)
	}

	sb := SecretBackend{
		ID:          o.ID,
		Name:        o.Name,
		BackendType: o.BackendType,
	}
	if o.TokenRotateInterval != nil {
		sb.TokenRotateInterval = database.NewNullDuration(*o.TokenRotateInterval)
	}

	err := tx.Query(ctx, o.upsertBackendStmt, sb).Run()
	if database.IsErrConstraintUnique(err) {
		return fmt.Errorf("%w: name %q", backenderrors.AlreadyExists, sb.Name)
	}
	if database.IsErrConstraintTrigger(err) {
		return fmt.Errorf("%w: %q is immutable", backenderrors.Forbidden, sb.ID)
	}
	if err != nil {
		return fmt.Errorf("cannot upsert secret backend %q: %w", o.Name, err)
	}
	if o.NextRotateTime != nil && !o.NextRotateTime.IsZero() {
		err = tx.Query(ctx, o.upsertRotationStmt, SecretBackendRotation{
			ID:               o.ID,
			NextRotationTime: sql.NullTime{Time: *o.NextRotateTime, Valid: true},
		}).Run()
		if err != nil {
			return fmt.Errorf("cannot upsert secret backend rotation time for %q: %w", o.Name, err)
		}
	}
	if len(o.Config) == 0 {
		return nil
	}
	namesToKeep := make(sqlair.S, 0, len(o.Config))
	for k := range o.Config {
		namesToKeep = append(namesToKeep, k)
	}
	err = tx.Query(ctx, o.clearConfigStmt, sqlair.M{"uuid": o.ID}, namesToKeep).Run()
	if err != nil {
		return fmt.Errorf("cannot clear secret backend config for %q: %w", o.Name, err)
	}
	for k, v := range o.Config {
		// TODO: this needs to be fix once the sqlair supports bulk insert.
		err = tx.Query(ctx, o.upsertConfigStmt, SecretBackendConfig{
			ID:      o.ID,
			Name:    k,
			Content: v.(string),
		}).Run()
		if err != nil {
			return fmt.Errorf("cannot upsert secret backend config for %q: %w", o.Name, err)
		}
	}
	return nil
}
