// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

func (s *State) upsertBackend(ctx context.Context, tx *sqlair.TX, sb SecretBackend) error {
	upsertBackendStmt, err := s.Prepare(`
INSERT INTO secret_backend
    (uuid, name, backend_type_id, token_rotate_interval)
VALUES ($SecretBackend.*)
ON CONFLICT (uuid) DO UPDATE SET
    name=EXCLUDED.name,
    token_rotate_interval=EXCLUDED.token_rotate_interval;`, SecretBackend{})
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, upsertBackendStmt, sb).Run()
	if database.IsErrConstraintUnique(err) {
		return errors.Errorf("%w: name %q", backenderrors.AlreadyExists, sb.Name)
	}
	if database.IsErrConstraintTrigger(err) {
		return errors.Errorf("%w: %q is immutable", backenderrors.Forbidden, sb.ID)
	}
	if err != nil {
		return errors.Errorf("cannot upsert secret backend %q: %w", sb.Name, err)
	}
	return nil
}

func (s *State) upsertBackendRotation(ctx context.Context, tx *sqlair.TX, r SecretBackendRotation) error {
	upsertRotationStmt, err := s.Prepare(`
INSERT INTO secret_backend_rotation
    (backend_uuid, next_rotation_time)
VALUES ($SecretBackendRotation.*)
ON CONFLICT (backend_uuid) DO UPDATE SET
    next_rotation_time=EXCLUDED.next_rotation_time;`,
		SecretBackendRotation{})
	if err != nil {
		return errors.Capture(err)
	}
	if err = tx.Query(ctx, upsertRotationStmt, r).Run(); err != nil {
		return errors.Errorf("cannot upsert secret backend rotation time for %q: %w", r.ID, err)
	}
	return nil
}

func (s *State) upsertBackendConfig(ctx context.Context, tx *sqlair.TX, id string, cfg map[string]string) error {
	clearConfigStmt, err := s.Prepare(`
DELETE FROM secret_backend_config
WHERE backend_uuid = $M.uuid AND name NOT IN ($S[:]);`,
		sqlair.M{}, sqlair.S{})
	if err != nil {
		return errors.Capture(err)
	}

	upsertConfigStmt, err := s.Prepare(`
INSERT INTO secret_backend_config
    (backend_uuid, name, content)
VALUES ($SecretBackendConfig.*)
ON CONFLICT (backend_uuid, name) DO UPDATE SET
    content = EXCLUDED.content`,
		SecretBackendConfig{})
	if err != nil {
		return errors.Capture(err)
	}

	namesToKeep := make(sqlair.S, 0, len(cfg))
	for k := range cfg {
		namesToKeep = append(namesToKeep, k)
	}
	if err = tx.Query(ctx, clearConfigStmt, sqlair.M{"uuid": id}, namesToKeep).Run(); err != nil {
		return errors.Errorf("cannot clear secret backend config for %q: %w", id, err)
	}
	for k, v := range cfg {
		// TODO: this needs to be fixed once the sqlair supports bulk insert.
		err = tx.Query(ctx, upsertConfigStmt, SecretBackendConfig{
			ID:      id,
			Name:    k,
			Content: v,
		}).Run()
		if err != nil {
			return errors.Errorf("cannot upsert secret backend config for %q: %w", id, err)
		}
	}
	return nil

}
