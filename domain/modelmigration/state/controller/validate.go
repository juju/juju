// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/internal/errors"
)

// These are the controller-database reads used by the migrationtarget
// VALIDATION phase (domain/modelmigration/service.ValidateImportedModel and the
// ActivateImport agent-binary check). They deliberately duplicate small
// existence queries owned by the secretbackend and agentbinary domains, keeping
// the validation a single domain concern next to the rest of the import code.

// GetKnownSecretBackends returns the subset of the supplied secret backend
// UUIDs that exist on the controller. It is used to detect model secret value
// refs that still carry a source-controller-local backend UUID after import.
func (s *State) GetKnownSecretBackends(ctx context.Context, uuids []string) ([]string, error) {
	if len(uuids) == 0 {
		return nil, nil
	}

	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT sb.uuid AS &entityUUID.uuid
FROM   secret_backend AS sb
WHERE  sb.uuid IN ($nameList[:])
`, entityUUID{}, nameList{})
	if err != nil {
		return nil, errors.Errorf("preparing known secret backends statement: %w", err)
	}

	var rows []entityUUID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		return getAll(ctx, tx, stmt, &rows, nameList(uuids))
	}); err != nil {
		return nil, errors.Errorf("retrieving known secret backends: %w", err)
	}

	known := make([]string, 0, len(rows))
	for _, r := range rows {
		known = append(known, r.UUID)
	}
	return known, nil
}

// GetSecretBackendReferencesForModel returns a map from secret revision UUID to
// the secret backend UUID recorded for it in secret_backend_reference for the
// given model. These are the re-attach rows the target must hold so external
// secret content resolves and backend ref-counting stays correct after import.
func (s *State) GetSecretBackendReferencesForModel(ctx context.Context, modelUUID string) (map[string]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	arg := modelUUIDArg{ModelUUID: modelUUID}
	stmt, err := s.Prepare(`
SELECT (secret_revision_uuid, secret_backend_uuid) AS (&secretBackendRef.*)
FROM   secret_backend_reference
WHERE  model_uuid = $modelUUIDArg.model_uuid
`, secretBackendRef{}, arg)
	if err != nil {
		return nil, errors.Errorf("preparing secret backend references statement: %w", err)
	}

	var rows []secretBackendRef
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		return getAll(ctx, tx, stmt, &rows, arg)
	}); err != nil {
		return nil, errors.Errorf("retrieving secret backend references for model %q: %w", modelUUID, err)
	}

	result := make(map[string]string, len(rows))
	for _, r := range rows {
		result[r.RevisionUUID] = r.BackendUUID
	}
	return result, nil
}

// GetAgentBinaryArchitecturesForVersion returns the architecture names for
// which the controller's object store holds agent binaries at the given
// version.
func (s *State) GetAgentBinaryArchitecturesForVersion(ctx context.Context, version string) ([]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	arg := agentVersionArg{Version: version}
	stmt, err := s.Prepare(`
SELECT a.name AS &architectureName.name
FROM   agent_binary_store AS abs
JOIN   architecture AS a ON a.id = abs.architecture_id
WHERE  abs.version = $agentVersionArg.version
`, architectureName{}, arg)
	if err != nil {
		return nil, errors.Errorf("preparing controller agent binary architectures statement: %w", err)
	}

	var rows []architectureName
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		return getAll(ctx, tx, stmt, &rows, arg)
	}); err != nil {
		return nil, errors.Errorf("retrieving controller agent binary architectures for version %q: %w", version, err)
	}

	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.Name)
	}
	return names, nil
}
