// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"strings"

	"github.com/canonical/sqlair"

	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
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

// GetRelationValidationData returns the relation identities and keys needed
// to validate imported relation-unit consistency before activation.
func (s *State) GetRelationValidationData(ctx context.Context) ([]modelmigrationinternal.RelationValidationData, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT (r.uuid, r.relation_id) AS (&relationValidationRow.*)
FROM   relation AS r
`, relationValidationRow{})
	if err != nil {
		return nil, errors.Errorf("preparing relation validation data statement: %w", err)
	}

	var rows []relationValidationRow
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	}); err != nil {
		return nil, errors.Errorf("retrieving relation validation data: %w", err)
	}

	relationKeys, err := s.getRelationKeys(ctx)
	if err != nil {
		return nil, errors.Errorf("retrieving relation keys: %w", err)
	}

	result := make([]modelmigrationinternal.RelationValidationData, len(rows))
	for i, row := range rows {
		result[i] = modelmigrationinternal.RelationValidationData{
			UUID: row.UUID,
			ID:   row.ID,
			Key:  strings.Join(relationKeys[row.UUID], " "),
		}
	}
	return result, nil
}

// relationEndpointKey maps a row from v_relation_endpoint_identifier into
// a relation's UUID, application and endpoint names.
type relationEndpointKey struct {
	RelationUUID    string `db:"relation_uuid"`
	ApplicationName string `db:"application_name"`
	EndpointName    string `db:"endpoint_name"`
}

// getRelationKeys returns a map from relation UUID to its endpoint
// application:endpoint pairs, used to build readable relation keys for
// validation error messages.
func (s *State) getRelationKeys(ctx context.Context) (map[string][]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT (relation_uuid, application_name, endpoint_name) AS (&relationEndpointKey.*)
FROM   v_relation_endpoint_identifier
`, relationEndpointKey{})
	if err != nil {
		return nil, errors.Errorf("preparing relation endpoint keys statement: %w", err)
	}

	var rows []relationEndpointKey
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	}); err != nil {
		return nil, errors.Errorf("retrieving relation endpoint keys: %w", err)
	}

	result := make(map[string][]string)
	for _, row := range rows {
		result[row.RelationUUID] = append(result[row.RelationUUID],
			row.ApplicationName+":"+row.EndpointName)
	}
	return result, nil
}

// GetApplicationUnitNames returns a map from application name to the names of
// its units, used to ensure every unit in a relation has a corresponding
// relation-unit row.
func (s *State) GetApplicationUnitNames(ctx context.Context) (map[string][]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT a.name AS &applicationUnitRow.application_name,
       u.name AS &applicationUnitRow.unit_name
FROM   application AS a
JOIN   unit AS u ON u.application_uuid = a.uuid
`, applicationUnitRow{})
	if err != nil {
		return nil, errors.Errorf("preparing application unit names statement: %w", err)
	}

	var rows []applicationUnitRow
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	}); err != nil {
		return nil, errors.Errorf("retrieving application unit names: %w", err)
	}

	result := make(map[string][]string)
	for _, row := range rows {
		result[row.ApplicationName] = append(result[row.ApplicationName], row.UnitName)
	}
	return result, nil
}

// GetRelationUnitsByApplication returns a map from relation UUID to the set of
// unit names in scope for that relation, grouped by application name.
func (s *State) GetRelationUnitsByApplication(ctx context.Context) (map[string]map[string][]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT re.relation_uuid AS &relationUnitScopeRow.relation_uuid,
       u.name AS &relationUnitScopeRow.unit_name,
       a.name AS &relationUnitScopeRow.application_name
FROM   relation_unit AS ru
JOIN   relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN   application AS a ON ae.application_uuid = a.uuid
JOIN   unit AS u ON ru.unit_uuid = u.uuid
`, relationUnitScopeRow{})
	if err != nil {
		return nil, errors.Errorf("preparing relation units by application statement: %w", err)
	}

	var rows []relationUnitScopeRow
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	}); err != nil {
		return nil, errors.Errorf("retrieving relation units by application: %w", err)
	}

	result := make(map[string]map[string][]string)
	for _, row := range rows {
		byApp, ok := result[row.RelationUUID]
		if !ok {
			byApp = make(map[string][]string)
			result[row.RelationUUID] = byApp
		}
		byApp[row.ApplicationName] = append(byApp[row.ApplicationName], row.UnitName)
	}
	return result, nil
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
