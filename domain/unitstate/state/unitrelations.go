// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/canonical/sqlair"

	coreerrors "github.com/juju/juju/core/errors"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/deployment/charm"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/unitstate/internal"
	"github.com/juju/juju/internal/errors"
)

// GetRegularRelationUUIDByEndpointIdentifiers gets the UUID of a regular
// relation specified by two endpoint identifiers.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if endpoints cannot be
//     found.
func (st *State) GetRegularRelationUUIDByEndpointIdentifiers(
	ctx context.Context,
	endpoint1, endpoint2 corerelation.EndpointIdentifier,
) (corerelation.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuid []entityUUID
	type endpointIdentifier1 endpointIdentifier
	type endpointIdentifier2 endpointIdentifier
	e1 := endpointIdentifier1{
		ApplicationName: endpoint1.ApplicationName,
		EndpointName:    endpoint1.EndpointName,
	}
	e2 := endpointIdentifier2{
		ApplicationName: endpoint2.ApplicationName,
		EndpointName:    endpoint2.EndpointName,
	}

	stmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   relation r
JOIN   v_relation_endpoint_identifier e1 ON r.uuid = e1.relation_uuid
JOIN   v_relation_endpoint_identifier e2 ON r.uuid = e2.relation_uuid
WHERE  e1.application_name = $endpointIdentifier1.application_name
AND    e1.endpoint_name    = $endpointIdentifier1.endpoint_name
AND    e2.application_name = $endpointIdentifier2.application_name
AND    e2.endpoint_name    = $endpointIdentifier2.endpoint_name
`, entityUUID{}, e1, e2)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, e1, e2).GetAll(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.RelationNotFound
		} else if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	if len(uuid) > 1 {
		return "", errors.Errorf("found multiple relations for endpoint pair")
	}

	return corerelation.UUID(uuid[0].UUID), nil
}

// GetPeerRelationUUIDByEndpointIdentifiers gets the UUID of a peer
// relation specified by a single endpoint identifier.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if endpoint cannot be
//     found.
func (st *State) GetPeerRelationUUIDByEndpointIdentifiers(
	ctx context.Context,
	endpoint corerelation.EndpointIdentifier,
) (corerelation.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	e := endpointIdentifier{
		ApplicationName: endpoint.ApplicationName,
		EndpointName:    endpoint.EndpointName,
	}

	stmt, err := st.Prepare(`
SELECT &relationUUIDAndRole.*
FROM   relation r
JOIN   v_relation_endpoint e ON r.uuid = e.relation_uuid
WHERE  e.application_name = $endpointIdentifier.application_name
AND    e.endpoint_name    = $endpointIdentifier.endpoint_name
`, relationUUIDAndRole{}, e)
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuidAndRole []relationUUIDAndRole
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, e).GetAll(&uuidAndRole)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.RelationNotFound
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	if len(uuidAndRole) > 1 {
		return "", errors.Errorf("found multiple relations for peer application endpoint combination")
	}

	// Verify that the role is peer. Endpoint names are unique per charm, so if
	// the role is not peer the application does not have a peer relation with
	// the specified endpoint name, so return RelationNotFound.
	if uuidAndRole[0].Role != string(charm.RolePeer) {
		return "", relationerrors.RelationNotFound
	}

	return corerelation.UUID(uuidAndRole[0].UUID), nil
}

// setRelationApplicationAndUnitSettings records settings for a unit and
// an application in a relation.
// Note: this method was copied from the relation domain state package and
// adapted. It retrieves the relation unit uuid, rather than the relation
// uuid based on known information when called.
func (st *State) setRelationApplicationAndUnitSettings(
	ctx context.Context, tx *sqlair.TX,
	unitUUID string,
	settings internal.RelationSettings,
) error {
	if len(settings.UnitSet) == 0 && len(settings.ApplicationSet) == 0 &&
		len(settings.UnitUnset) == 0 && len(settings.ApplicationUnset) == 0 {
		return nil
	}

	relationUnitUUID, applicationUUID, err := st.getRelationUnitAndApplication(
		ctx, tx, unitUUID, settings.RelationUUID.String(),
	)
	if err != nil {
		return err
	}

	err = st.setRelationUnitSettings(ctx, tx, relationUnitUUID, settings.UnitSet, settings.UnitUnset)
	if err != nil {
		return err
	}

	err = st.setRelationApplicationSettings(
		ctx, tx,
		settings.RelationUUID.String(),
		applicationUUID,
		settings.ApplicationSet,
		settings.ApplicationUnset,
	)
	if err != nil {
		return errors.Errorf("setting relation application settings: %w", err)
	}

	return nil
}

func (st *State) setRelationUnitSettings(
	ctx context.Context,
	tx *sqlair.TX,
	relationUnitUUID string,
	settings map[string]string,
	unset keys,
) error {
	if len(settings) == 0 && len(unset) == 0 {
		return nil
	}

	// Update the unit settings specified in the settings argument.
	err := st.updateUnitSettings(ctx, tx, relationUnitUUID, settings, unset)
	if err != nil {
		return errors.Errorf("updating relation unit settings: %w", err)
	}

	// Fetch all the new settings in the relation for this unit.
	newSettings, err := st.getRelationUnitSettings(ctx, tx, relationUnitUUID)
	if err != nil {
		return errors.Errorf("getting new relation unit settings: %w", err)
	}

	// Hash the new settings.
	hash, err := hashSettings(newSettings)
	if err != nil {
		return errors.Errorf("generating hash of relation unit settings: %w", err)
	}

	// Update the hash in the database.
	err = st.updateUnitSettingsHash(ctx, tx, relationUnitUUID, hash)
	if err != nil {
		return err
	}

	return nil
}

func (st *State) setRelationApplicationSettings(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID string,
	applicationID string,
	settings map[string]string,
	unset keys,
) error {
	if len(settings) == 0 && len(unset) == 0 {
		return nil
	}

	// Get the relation endpoint UUID.
	endpointUUID, err := st.getRelationEndpointUUID(ctx, tx, relationUUID, applicationID)
	if err != nil {
		return errors.Errorf("getting relation endpoint uuid: %w", err)
	}

	// Update the application settings specified in the settings argument.
	err = st.updateApplicationSettings(ctx, tx, endpointUUID, settings, unset)
	if err != nil {
		return errors.Errorf("updating relation application settings: %w", err)
	}

	// Fetch all the new settings in the relation for this application.
	newSettings, err := st.getApplicationSettings(ctx, tx, endpointUUID)
	if err != nil {
		return errors.Errorf("getting new relation application settings: %w", err)
	}

	// Hash the new settings.
	hash, err := hashSettings(newSettings)
	if err != nil {
		return errors.Errorf("generating hash of relation application settings: %w", err)
	}

	// Update the hash in the database.
	err = st.updateApplicationSettingsHash(ctx, tx, endpointUUID, hash)
	if err != nil {
		return errors.Errorf("updating relation application settings hash: %w", err)
	}

	return nil
}

// updateUnitSettings updates the settings for a relation unit according to the
// provided settings map. If the value of a setting is empty then the setting is
// deleted, otherwise it is inserted/updated.
func (st *State) updateUnitSettings(
	ctx context.Context, tx *sqlair.TX, relUnitUUID string, settings map[string]string, unset keys,
) error {
	// Determine the keys to set and unset.
	var set []relationUnitSetting
	for k, v := range settings {
		set = append(set, relationUnitSetting{
			UUID:  relUnitUUID,
			Key:   k,
			Value: v,
		})
	}

	// Update the keys to set.
	if len(set) > 0 {
		updateStmt, err := st.Prepare(`
INSERT INTO relation_unit_setting (*)
VALUES ($relationUnitSetting.*)
ON CONFLICT (relation_unit_uuid, key) DO UPDATE SET value = excluded.value
`, relationUnitSetting{})
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, updateStmt, set).Run()
		if err != nil {
			return errors.Capture(err)
		}
	}

	// Delete the keys to unset.
	if len(unset) > 0 {
		id := entityUUID{UUID: relUnitUUID}
		deleteStmt, err := st.Prepare(`
DELETE FROM relation_unit_setting
WHERE       relation_unit_uuid = $entityUUID.uuid
AND         key IN ($keys[:])
`, id, unset)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, deleteStmt, id, unset).Run()
		if err != nil {
			return errors.Capture(err)
		}
	}

	return nil
}

func (st *State) updateUnitSettingsHash(ctx context.Context, tx *sqlair.TX, unitUUID string, hash string) error {
	arg := unitSettingsHash{
		RelationUnitUUID: unitUUID,
		Hash:             hash,
	}
	stmt, err := st.Prepare(`
INSERT INTO relation_unit_settings_hash (*)
VALUES ($unitSettingsHash.*)
ON CONFLICT (relation_unit_uuid) DO UPDATE SET sha256 = excluded.sha256
`, unitSettingsHash{})
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, arg).Run()
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) getRelationEndpointUUID(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID string,
	applicationID string,
) (string, error) {
	id := relationAndApplicationUUID{
		RelationUUID:  relationUUID,
		ApplicationID: applicationID,
	}
	var endpointUUID entityUUID
	stmt, err := st.Prepare(`
SELECT re.uuid AS &entityUUID.uuid
FROM   application_endpoint ae
JOIN   relation_endpoint re ON re.endpoint_uuid = ae.uuid
WHERE  ae.application_uuid = $relationAndApplicationUUID.application_uuid
AND    re.relation_uuid = $relationAndApplicationUUID.relation_uuid
`, id, endpointUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, id).Get(&endpointUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	return endpointUUID.UUID, nil
}

func (st *State) getRelationUnitSettings(
	ctx context.Context, tx *sqlair.TX, relUnitUUID string,
) ([]relationSetting, error) {
	id := entityUUID{UUID: relUnitUUID}
	stmt, err := st.Prepare(`
SELECT   &relationSetting.*
FROM     relation_unit_setting
WHERE    relation_unit_uuid = $entityUUID.uuid
ORDER BY key ASC
`, id, relationSetting{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var settings []relationSetting
	err = tx.Query(ctx, stmt, id).GetAll(&settings)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}
	return settings, nil
}

func hashSettings(settings []relationSetting) (string, error) {
	h := sha256.New()

	for _, s := range settings {
		if _, err := h.Write([]byte(s.Key + " " + s.Value + " ")); err != nil {
			return "", errors.Errorf("writing relation setting: %w", err)
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func (st *State) getApplicationSettings(
	ctx context.Context,
	tx *sqlair.TX,
	endpointUUID string,
) ([]relationSetting, error) {
	id := entityUUID{UUID: endpointUUID}
	stmt, err := st.Prepare(`
SELECT   &relationSetting.*
FROM     relation_application_setting
WHERE    relation_endpoint_uuid = $entityUUID.uuid
ORDER BY key ASC
`, id, relationSetting{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var settings []relationSetting
	err = tx.Query(ctx, stmt, id).GetAll(&settings)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	return settings, nil
}

// updateApplicationSettings updates the settings for a relation endpoint
// according to the provided settings map. If the value of a setting is empty
// then the setting is deleted, otherwise it is inserted/updated.
func (st *State) updateApplicationSettings(
	ctx context.Context,
	tx *sqlair.TX,
	endpointUUID string,
	settings map[string]string,
	unset keys,
) error {
	if len(settings) == 0 && len(unset) == 0 {
		return nil
	}

	// Determine the keys to set and unset.
	var set []relationApplicationSetting
	for k, v := range settings {
		set = append(set, relationApplicationSetting{
			UUID:  endpointUUID,
			Key:   k,
			Value: v,
		})
	}

	// Update the keys to set.
	if len(set) > 0 {
		updateStmt, err := st.Prepare(`
INSERT INTO relation_application_setting (*) 
VALUES ($relationApplicationSetting.*) 
ON CONFLICT (relation_endpoint_uuid, key) DO UPDATE SET value = excluded.value
`, relationApplicationSetting{})
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, updateStmt, set).Run()
		if err != nil {
			return errors.Capture(err)
		}
	}

	// Delete the keys to unset.
	if len(unset) > 0 {
		id := entityUUID{UUID: endpointUUID}
		deleteStmt, err := st.Prepare(`
DELETE FROM relation_application_setting
WHERE       relation_endpoint_uuid = $entityUUID.uuid
AND         key IN ($keys[:])
`, id, unset)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, deleteStmt, id, unset).Run()
		if err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

func (st *State) updateApplicationSettingsHash(
	ctx context.Context,
	tx *sqlair.TX,
	endpointUUID, hash string,
) error {
	arg := applicationSettingsHash{
		RelationEndpointUUID: endpointUUID,
		Hash:                 hash,
	}
	stmt, err := st.Prepare(`
INSERT INTO relation_application_settings_hash (*) 
VALUES ($applicationSettingsHash.*) 
ON CONFLICT (relation_endpoint_uuid) DO UPDATE SET sha256 = excluded.sha256
`, applicationSettingsHash{})
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, arg).Run()
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) getRelationUnitAndApplication(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID string,
	relationUUID string,
) (relationUnitUUID string, applicationUUID string, err error) {
	stmt, prepErr := st.Prepare(`
SELECT   ru.uuid AS &relationUnitAndApp.relation_unit_uuid,
         a.uuid  AS &relationUnitAndApp.application_uuid
FROM     relation_unit AS ru
JOIN     relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
JOIN     relation AS r ON re.relation_uuid = r.uuid
JOIN     unit AS u ON ru.unit_uuid = u.uuid
JOIN     application AS a ON u.application_uuid = a.uuid
WHERE    ru.unit_uuid = $getUnitRelAndApp.unit_uuid
AND      r.uuid = $getUnitRelAndApp.relation_uuid
`, relationUnitAndApp{}, getUnitRelAndApp{})
	if prepErr != nil {
		return "", "", errors.Errorf("preparing relation unit/application query: %w", prepErr)
	}

	row := relationUnitAndApp{}
	err = tx.Query(ctx, stmt, getUnitRelAndApp{UnitUUID: unitUUID, RelationUUID: relationUUID}).Get(&row)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", "", errors.Errorf(
			"relation unit/application not found for unit %q and relation %q",
			unitUUID, relationUUID,
		).Add(coreerrors.NotFound)
	}
	if err != nil {
		return "", "", errors.Errorf(
			"querying relation unit/application for unit %q and relation %q: %w",
			unitUUID, relationUUID, err,
		)
	}

	return row.RelationUnitUUID, row.ApplicationUUID, nil
}
