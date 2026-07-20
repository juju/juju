// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/internal/errors"
)

// These are the model-database reads used by the migrationtarget VALIDATION
// phase (domain/modelmigration/service.ValidateImportedModel, the ActivateImport
// agent-binary check, and CheckMachines). They run over the imported,
// still-gated model before activation clears the model_migrating gate.

// GetMachineInstanceIDs returns a map from provider cloud instance ID to the
// name of the model machine it backs, for every provisioned machine. It is used
// by CheckMachines to reconcile the model's machines against the instances the
// provider reports, naming the offending machine for each discrepancy.
func (s *State) GetMachineInstanceIDs(ctx context.Context) (map[string]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT (m.name, mci.instance_id) AS (&machineInstanceID.*)
FROM   machine_cloud_instance AS mci
JOIN   machine AS m ON m.uuid = mci.machine_uuid
`, machineInstanceID{})
	if err != nil {
		return nil, errors.Errorf("preparing machine instance IDs statement: %w", err)
	}

	var rows []machineInstanceID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	}); err != nil {
		return nil, errors.Errorf("retrieving machine instance IDs: %w", err)
	}

	result := make(map[string]string, len(rows))
	for _, r := range rows {
		result[r.InstanceID] = r.MachineName
	}
	return result, nil
}

// GetModelType returns the model's deployment type (for example "iaas" or
// "caas").
func (s *State) GetModelType(ctx context.Context) (string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	stmt, err := s.Prepare(`SELECT &modelType.type FROM model`, modelType{})
	if err != nil {
		return "", errors.Errorf("preparing get model type statement: %w", err)
	}

	var result modelType
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("model information is missing from database")
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("getting model type: %w", err)
	}
	return result.Type, nil
}

// GetSecretBackendUUIDsInUse returns the distinct secret backend UUIDs
// referenced by the model's external secret value refs, including deleted value
// refs whose external content is pending cleanup. These are the backend UUIDs
// that must resolve to a real secret backend on the target controller.
func (s *State) GetSecretBackendUUIDsInUse(ctx context.Context) ([]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
WITH used_backend AS (
    SELECT backend_uuid FROM secret_value_ref
    UNION
    SELECT backend_uuid FROM secret_deleted_value_ref
)
SELECT ub.backend_uuid AS &secretBackendUUID.backend_uuid
FROM   used_backend AS ub
`, secretBackendUUID{})
	if err != nil {
		return nil, errors.Errorf("preparing secret backends in use statement: %w", err)
	}

	var rows []secretBackendUUID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	}); err != nil {
		return nil, errors.Errorf("retrieving secret backends in use: %w", err)
	}

	backends := make([]string, 0, len(rows))
	for _, r := range rows {
		backends = append(backends, r.BackendUUID)
	}
	return backends, nil
}

// GetExternalSecretRevisionBackends returns a map from secret revision UUID to
// the backend UUID its external value ref points at, for every revision whose
// content is stored externally (i.e. that has a secret_value_ref row). Revisions
// whose content is stored in the model database (secret_content) are not
// included: only externally backed revisions require backend re-attachment.
func (s *State) GetExternalSecretRevisionBackends(ctx context.Context) (map[string]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT (revision_uuid, backend_uuid) AS (&revisionBackend.*)
FROM   secret_value_ref
`, revisionBackend{})
	if err != nil {
		return nil, errors.Errorf("preparing external secret revision backends statement: %w", err)
	}

	var rows []revisionBackend
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	}); err != nil {
		return nil, errors.Errorf("retrieving external secret revision backends: %w", err)
	}

	result := make(map[string]string, len(rows))
	for _, r := range rows {
		result[r.RevisionUUID] = r.BackendUUID
	}
	return result, nil
}

// GetRunningAgentArchitectures returns the distinct architecture names reported
// by the model's machine and unit agents. These are the architectures for which
// the target must have agent binaries at the desired version before the agents
// are bumped to it.
func (s *State) GetRunningAgentArchitectures(ctx context.Context) ([]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
WITH running_arch AS (
    SELECT architecture_id FROM machine_agent_version
    UNION
    SELECT architecture_id FROM unit_agent_version
)
SELECT a.name AS &architectureName.name
FROM   running_arch AS ra
JOIN   architecture AS a ON a.id = ra.architecture_id
`, architectureName{})
	if err != nil {
		return nil, errors.Errorf("preparing running agent architectures statement: %w", err)
	}

	var rows []architectureName
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	}); err != nil {
		return nil, errors.Errorf("retrieving running agent architectures: %w", err)
	}

	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.Name)
	}
	return names, nil
}

// GetAgentBinaryArchitecturesForVersion returns the architecture names for
// which the model's object store holds agent binaries at the given version.
func (s *State) GetAgentBinaryArchitecturesForVersion(ctx context.Context, version string) ([]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	arg := versionArg{Version: version}
	stmt, err := s.Prepare(`
SELECT a.name AS &architectureName.name
FROM   agent_binary_store AS abs
JOIN   architecture AS a ON a.id = abs.architecture_id
WHERE  abs.version = $versionArg.version
`, architectureName{}, arg)
	if err != nil {
		return nil, errors.Errorf("preparing model agent binary architectures statement: %w", err)
	}

	var rows []architectureName
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		err := tx.Query(ctx, stmt, arg).GetAll(&rows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	}); err != nil {
		return nil, errors.Errorf("retrieving model agent binary architectures for version %q: %w", version, err)
	}

	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.Name)
	}
	return names, nil
}
