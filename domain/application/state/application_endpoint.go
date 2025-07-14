// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"
	"slices"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/errors"
)

// GetAllEndpointBindings returns the all endpoint bindings for the model, where
// endpoints are indexed by the application name for the application which they
// belong to.
func (st *State) GetAllEndpointBindings(ctx context.Context) (map[string]map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	endpointBindingsStmt, err := st.Prepare(`
SELECT a.name AS &applicationEndpointBinding.application_name,
       ae.space_uuid AS &applicationEndpointBinding.space_uuid,
       cr.name AS &applicationEndpointBinding.endpoint_name
FROM   application_endpoint ae
JOIN   charm_relation cr ON cr.uuid = ae.charm_relation_uuid
JOIN   application a ON a.uuid = ae.application_uuid
`, applicationEndpointBinding{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	extraEndpointBindingsStmt, err := st.Prepare(`
SELECT a.name AS &applicationEndpointBinding.application_name,
       aee.space_uuid AS &applicationEndpointBinding.space_uuid,
       ceb.name AS &applicationEndpointBinding.endpoint_name
FROM   application_extra_endpoint aee
JOIN   charm_extra_binding ceb ON ceb.uuid = aee.charm_extra_binding_uuid
JOIN   application a ON a.uuid = aee.application_uuid
`, applicationEndpointBinding{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	defaultSpacesStmt, err := st.Prepare(`SELECT &applicationSpaceUUID.* FROM application`, applicationSpaceUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	spacesStmt, err := st.Prepare(`SELECT &space.* FROM space`, space{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var allEndpointBindings []applicationEndpointBinding
	var defaultSpaces []applicationSpaceUUID
	var allSpaces []space
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var endpointBindings []applicationEndpointBinding
		err := tx.Query(ctx, endpointBindingsStmt).GetAll(&endpointBindings)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting all endpoint bindings: %w", err)
		}

		var extraEndpointBindings []applicationEndpointBinding
		err = tx.Query(ctx, extraEndpointBindingsStmt).GetAll(&extraEndpointBindings)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting all endpoint bindings: %w", err)
		}

		err = tx.Query(ctx, defaultSpacesStmt).GetAll(&defaultSpaces)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting all default spaces: %w", err)
		}

		err = tx.Query(ctx, spacesStmt).GetAll(&allSpaces)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting all spaces: %w", err)
		}

		allEndpointBindings = append(endpointBindings, extraEndpointBindings...)
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	spaceUUIDToName := make(map[string]string)
	for _, space := range allSpaces {
		spaceUUIDToName[space.UUID] = space.Name
	}

	result := make(map[string]map[string]string)
	for _, space := range defaultSpaces {
		if _, ok := result[space.ApplicationName]; !ok {
			result[space.ApplicationName] = make(map[string]string)
		}
		spaceName, ok := spaceUUIDToName[space.SpaceUUID]
		if !ok {
			return nil, errors.Errorf("space %q not found", space.SpaceUUID)
		}
		result[space.ApplicationName][""] = spaceName
	}

	for _, binding := range allEndpointBindings {

		if binding.SpaceUUID.Valid {
			spaceName, ok := spaceUUIDToName[binding.SpaceUUID.V]
			if !ok {
				return nil, errors.Errorf("space %q not found", binding.SpaceUUID.V)
			}
			result[binding.ApplicationName][binding.EndpointName] = spaceName
		} else {
			appDefaultSpace, ok := result[binding.ApplicationName][""]
			if !ok {
				return nil, errors.Errorf("no default space found for application %q", binding.ApplicationName)
			}
			result[binding.ApplicationName][binding.EndpointName] = appDefaultSpace
		}
	}
	return result, nil
}

// GetApplicationEndpointBindings returns the mapping for each endpoint name and
// the space ID it is bound to (or empty if unspecified). When no bindings are
// stored for the application, defaults are returned.
//
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationEndpointBindings(ctx context.Context, appUUID coreapplication.ID) (map[string]network.SpaceUUID, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result map[string]network.SpaceUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		result, err = st.getEndpointBindings(ctx, tx, appUUID)
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return result, nil
}

// GetApplicationsBoundToSpace returns the names of the applications bound to
// the given space.
func (st *State) GetApplicationsBoundToSpace(ctx context.Context, uuid string) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := spaceUUID{UUID: uuid}

	// an application endpoint is bound to a space if:
	// - the endpoint space_uuid points explicitly to the space or
	// - the space_uuid is null and the application default space is that space
	bindingsStmt, err := st.Prepare(`
SELECT name AS &applicationName.name FROM (
    SELECT a.name
        FROM  application AS a
        JOIN  application_endpoint AS ae ON a.uuid = ae.application_uuid
        WHERE ae.space_uuid = $spaceUUID.uuid
        OR    ae.space_uuid IS NULL AND a.space_uuid = $spaceUUID.uuid
    UNION
    SELECT a.name
        FROM  application AS a
        JOIN  application_extra_endpoint AS aee ON a.uuid = aee.application_uuid
        WHERE aee.space_uuid = $spaceUUID.uuid
        OR    aee.space_uuid IS NULL AND a.space_uuid = $spaceUUID.uuid
)
`, applicationName{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var applications []applicationName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, bindingsStmt, ident).GetAll(&applications)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting applications bound to space %q: %w", uuid, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.Slice(applications, func(a applicationName) string { return a.Name }), nil
}

// GetApplicationEndpointNames returns the names of the endpoints for the given
// application.
// The following errors may be returned:
//   - [applicationerrors.ApplicationNotFound] is returned if the application
//     doesn't exist.
func (st *State) GetApplicationEndpointNames(ctx context.Context, appUUID coreapplication.ID) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var eps []charmRelationName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		charmUUID, err := st.getCharmIDByApplicationID(ctx, tx, appUUID)
		if err != nil {
			return errors.Errorf("getting charm for application %q: %w", appUUID, err)
		}
		eps, err = st.getCharmRelationNames(ctx, tx, charmID{UUID: charmUUID})
		if err != nil {
			return errors.Errorf("getting endpoint names for application %q: %w", appUUID, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.Slice(eps, func(r charmRelationName) string { return r.Name }), nil
}

// MergeApplicationEndpointBindings merges the provided bindings into the bindings
// for the specified application.
// The following errors may be returned:
func (st *State) MergeApplicationEndpointBindings(ctx context.Context, appID string, bindings map[string]string, force bool) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.mergeApplicationEndpointBindings(ctx, tx, appID, bindings, force)
		return errors.Capture(err)
	})
}

func (st *State) mergeApplicationEndpointBindings(ctx context.Context, tx *sqlair.TX, appID string, bindings map[string]string, force bool) error {
	bindingTables, err := st.getBindingTableTypes(ctx, tx, appID, slices.Collect(maps.Keys(bindings)))
	if err != nil {
		return errors.Capture(err)
	}

	spacesToUUIDs, err := st.getApplicationEndpointSpaceUUIDs(ctx, tx, appID, slices.Collect(maps.Values(bindings)))
	if err != nil {
		return errors.Capture(err)
	}

	validateErr := st.validateUnitsInSpaces(ctx, tx, appID, slices.Collect(maps.Values(spacesToUUIDs)))
	if !force && validateErr != nil {
		return errors.Capture(err)
	} else if force && validateErr != nil {
		st.logger.Infof(ctx, "binding validation ignored due to force: %w", validateErr)
	}

	// TODO: update updateDefaultSpace to take a map[string]string.
	// There is no value add to have them as SpaceNames here.
	spaceNameBindings := transform.Map(bindings, func(k, v string) (string, network.SpaceName) {
		return k, network.SpaceName(v)
	})

	if err := st.updateDefaultSpace(ctx, tx, appID, spaceNameBindings); err != nil {
		return errors.Errorf("updating default space: %w", err)
	}

	if err := st.updateValidatedApplicationEndpointSpaces(ctx, tx, appID, bindings, spacesToUUIDs, bindingTables); err != nil {
		return errors.Capture(err)
	}

	return nil

}

// insertApplicationEndpointsParams contains parameters required to insert
// application endpoints into the database.
type insertApplicationEndpointsParams struct {
	appID coreapplication.ID

	// EndpointBindings is a map to bind application endpoint by name to a
	// specific space. The default space is referenced by an empty key, if any.
	bindings map[string]network.SpaceName
}

// insertApplicationEndpointBindings inserts database records for an
// application's endpoints (`application_endpoint` and
// `application_extra_endpoint`).
//
// It gets the relation and extra binding defined in the charm and resolve
// optional bindings.
// Bindings needs to refer to existing spaces, charm relation and extra binding.
//
// After the insertion, the application would be linked to its
// `application_endpoint` and `application_extra_endpoint`, each of those will
// have an optional space defined (if there have been a related binding), and
// the application default space may have been updated if a binding without endpoint
// was present in params.
func (st *State) insertApplicationEndpointBindings(ctx context.Context, tx *sqlair.TX, params insertApplicationEndpointsParams) error {
	charm, err := st.getCharmIDByApplicationID(ctx, tx, params.appID)
	if err != nil {
		return errors.Capture(err)
	}
	charmUUID := charmID{UUID: charm}

	// Get charm relation.
	relations, err := st.getCharmRelationNames(ctx, tx, charmUUID)
	if err != nil {
		return errors.Errorf("getting charm relation names: %w", err)
	}

	// Get extra bindings
	extrabindings, err := st.getCharmExtraBindings(ctx, tx, charmUUID)
	if err != nil {
		return errors.Errorf("getting charm extra bindings: %w", err)
	}

	// Check that spaces are valid in binding.
	spaceNamesToUUID, err := st.checkSpaceNames(ctx, tx, slices.Collect(maps.Values(params.bindings)))
	if err != nil {
		return errors.Errorf("checking space names: %w", err)
	}
	// Check that binding are linked to valid endpoint (either from a charm relation
	// or an extra binding.
	if err := st.checkEndpointBindingName(relations, extrabindings, params.bindings); err != nil {
		return errors.Errorf("checking charm relation: %w", err)
	}

	// Insert endpoints.
	if err := st.insertApplicationRelationEndpointBindings(ctx, tx, params.appID, relations, spaceNamesToUUID, params.bindings); err != nil {
		return errors.Errorf("inserting application endpoint: %w", err)
	}

	// Insert extra bindings.
	if err := st.insertApplicationExtraBindings(ctx, tx, params.appID, extrabindings, spaceNamesToUUID, params.bindings); err != nil {
		return errors.Errorf("inserting application endpoint: %w", err)
	}

	return nil
}

// insertApplicationRelationEndpointBindings inserts an application endpoint
// binding into the database, associating it with a relation and space.
func (st *State) insertApplicationRelationEndpointBindings(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	relations []charmRelationName,
	spaceNamesToUUID map[network.SpaceName]string,
	bindings map[string]network.SpaceName,
) error {
	if len(relations) == 0 {
		return nil
	}

	insertApplicationEndpointStmt, err := st.Prepare(
		`INSERT INTO application_endpoint (*) VALUES ($setApplicationEndpointBinding.*)`,
		setApplicationEndpointBinding{},
	)
	if err != nil {
		return errors.Errorf("preparing insert application endpoint bindings: %w", err)
	}

	inserts := make([]setApplicationEndpointBinding, len(relations))
	for i, relation := range relations {
		uuid, err := corerelation.NewEndpointUUID()
		if err != nil {
			return errors.Capture(err)
		}
		// If this endpoint does not have an explicit binding, or is bound to
		// the default space "", insert a null value for the space uuid.
		space := sql.Null[string]{}
		if spaceName, ok := bindings[relation.Name]; ok && spaceName != "" {
			spaceUUID, ok := spaceNamesToUUID[spaceName]
			if !ok {
				return errors.Errorf("space %q not found", spaceName)
			}
			space = sql.Null[string]{
				V:     spaceUUID,
				Valid: true,
			}
		}
		inserts[i] = setApplicationEndpointBinding{
			UUID:          uuid,
			ApplicationID: appID,
			RelationUUID:  relation.UUID,
			Space:         space,
		}
	}

	return tx.Query(ctx, insertApplicationEndpointStmt, inserts).Run()
}

// insertApplicationExtraBindings inserts a charm extra binding into the database,
// associating it with a relation and space.
func (st *State) insertApplicationExtraBindings(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	extraBindings []charmExtraBinding,
	spaceNamesToUUID map[network.SpaceName]string,
	bindings map[string]network.SpaceName,
) error {
	if len(extraBindings) == 0 {
		return nil
	}

	insertStmt, err := st.Prepare(
		`INSERT INTO application_extra_endpoint (*) VALUES ($setApplicationExtraEndpointBinding.*)`,
		setApplicationExtraEndpointBinding{},
	)
	if err != nil {
		return errors.Errorf("preparing insert application extra endpoint bindings: %w", err)
	}

	inserts := make([]setApplicationExtraEndpointBinding, len(extraBindings))
	for i, extraBinding := range extraBindings {
		// If this endpoint does not have an explicit binding, or is bound to
		// the default space "", insert a null value for the space uuid.
		space := sql.Null[string]{}
		if spaceName, ok := bindings[extraBinding.Name]; ok && spaceName != "" {
			spaceUUID, ok := spaceNamesToUUID[spaceName]
			if !ok {
				return errors.Errorf("space %q not found", spaceName)
			}
			space = sql.Null[string]{
				V:     spaceUUID,
				Valid: true,
			}
		}
		inserts[i] = setApplicationExtraEndpointBinding{
			ApplicationID: appID,
			RelationUUID:  extraBinding.UUID,
			Space:         space,
		}
	}

	return tx.Query(ctx, insertStmt, inserts).Run()
}

func (st *State) validateUnitsInSpaces(
	ctx context.Context,
	tx *sqlair.TX,
	appID string,
	spaceUUIDs []string,
) error {
	type spaces []string

	stmt, err := st.Prepare(`
WITH
-- actual unit space permutations for the application
app_unit_spaces AS (
    SELECT u.uuid AS unit_uuid,
           u.name AS unit_name,
           sn.space_uuid AS space_uuid,
           s.name AS space_name
    FROM   unit AS u
    JOIN   net_node AS nn ON u.net_node_uuid = nn.uuid
    JOIN   link_layer_device AS lld ON nn.uuid = lld.net_node_uuid
    JOIN   ip_address AS ip ON lld.uuid = ip.device_uuid
    JOIN   subnet AS sn ON ip.subnet_uuid = sn.uuid
    JOIN   space AS s ON sn.space_uuid = s.uuid
    WHERE  u.application_uuid = $dbUUID.uuid
),
-- desired unit space permutations
desired_unit_spaces AS (  
    SELECT u.uuid AS unit_uuid,
           u.name AS unit_name, 
           s.uuid AS space_uuid,
           s.name AS space_name
    FROM unit AS u
    LEFT JOIN space AS s ON s.uuid IN ($spaces[:])
    WHERE u.application_uuid = $dbUUID.uuid
),
diff_spaces AS (
    SELECT * FROM desired_unit_spaces
    EXCEPT
    SELECT * FROM app_unit_spaces
)
SELECT &unitSpaceName.* FROM diff_spaces
`, dbUUID{}, spaces{}, unitSpaceName{})
	if err != nil {
	}

	var unitSpaceFailure []unitSpaceName
	err = tx.Query(ctx, stmt, dbUUID{UUID: appID}, spaces(spaceUUIDs)).GetAll(&unitSpaceFailure)
	if errors.Is(err, sqlair.ErrNoRows) {
		// No Rows is what we are looking for.
		return nil
	} else if err != nil {
		return errors.Capture(err)
	}

	// Make a pretty error message with the result.
	comboErr := make([]error, len(unitSpaceFailure))
	for i, fail := range unitSpaceFailure {
		comboErr[i] = errors.Errorf("unit %q not in space %q", fail.UnitName, fail.SpaceName)
	}
	return errors.Join(comboErr...)
}

// updateValidatedApplicationEndpointSpaces updates the space UUIDs of the
// provided application's endpoints. It is required that the endpoint names
// and spaces have previously been validated.
func (st *State) updateValidatedApplicationEndpointSpaces(
	ctx context.Context,
	tx *sqlair.TX,
	appID string,
	bindings, spacesToUUIDs map[string]string,
	bindingTypes map[string]bindingToTable,
) error {
	// The default space for the application was updated in updateDefaultSpace.
	// The default space is represented by an empty string, thus cannot be found
	// in the database. Do not try to update it.
	bindingsToFind := make(map[string]string, 0)
	for k, v := range bindings {
		if k == "" {
			continue
		}
		bindingsToFind[k] = v
	}
	if len(bindingTypes) == 0 {
		return nil
	}
	for binding, spaceName := range bindingsToFind {
		var spaceUUID sql.Null[string]
		uuid, _ := spacesToUUIDs[spaceName]
		if spaceName != "" {
			spaceUUID = sql.Null[string]{
				V:     uuid,
				Valid: true,
			}
		}
		bindingType, _ := bindingTypes[binding]
		err := st.updateApplicationEndpointSpace(ctx, tx, appID, bindingType, spaceUUID)
		if err != nil {
			return errors.Errorf("failure to update endpoint %q: %w", bindingType.Name, err)
		}
	}

	return nil
}

// updateApplicationEndpointSpace updates the space uuid of the binding
// be it a relation or extra binding. A row will only be updated if the
// application's units have devices in the given space.
func (st *State) updateApplicationEndpointSpace(
	ctx context.Context,
	tx *sqlair.TX,
	appID string,
	binding bindingToTable,
	spaceUUID sql.Null[string],
) error {
	var query string
	switch binding.BindingType {
	case BindingRelation:
		query = `
UPDATE application_endpoint
SET    space_uuid = $updateBinding.space_uuid
WHERE  application_uuid = $updateBinding.application_uuid
AND    charm_relation_uuid = $updateBinding.binding_uuid
`
	case BindingExtra:
		query = `
UPDATE application_extra_endpoint
SET    space_uuid = $updateBinding.space_uuid
WHERE  application_uuid = $updateBinding.application_uuid
AND    charm_extra_binding_uuid = $updateBinding.binding_uuid
`
	default:
		return errors.Errorf("programming error, invalid endpoint type")
	}

	updateEndpointStmt, err := st.Prepare(query, updateBinding{})
	if err != nil {
		return errors.Errorf("preparing update application endpoint: %w", err)
	}

	update := updateBinding{
		ApplicationID: appID,
		BindingUUID:   binding.UUID,
		Space:         spaceUUID,
	}

	return tx.Query(ctx, updateEndpointStmt, update).Run()
}

// getCharmRelationNames retrieves a list of charm relation names from the
// database based on the provided parameters.
func (st *State) getCharmRelationNames(ctx context.Context, tx *sqlair.TX,
	charmUUID charmID) ([]charmRelationName,
	error) {
	fetchCharmRelationStmt, err := st.Prepare(`
SELECT &charmRelationName.* 
FROM charm_relation
WHERE charm_relation.charm_uuid = $charmID.uuid
`, charmUUID, charmRelationName{})
	if err != nil {
		return nil, errors.Errorf("preparing fetch charm relation: %w", err)
	}
	var relations []charmRelationName
	if err := tx.Query(ctx, fetchCharmRelationStmt, charmUUID).GetAll(&relations); err != nil && !errors.Is(err,
		sqlair.ErrNoRows) {
		return nil, errors.Errorf("fetching charm relation: %w", err)
	}
	return relations, nil
}

// checkSpaceNames verifies that all provided network space names exist in the
// database and returns a map from the space names to their UUIDs.
func (st *State) checkSpaceNames(ctx context.Context, tx *sqlair.TX, inputs []network.SpaceName) (map[network.SpaceName]string, error) {
	fetchStmt, err := st.Prepare(`
SELECT &space.*
FROM space`, space{})
	if err != nil {
		return nil, errors.Errorf("preparing fetch space: %w", err)
	}
	var spaces []space
	if err := tx.Query(ctx, fetchStmt).GetAll(&spaces); err != nil {
		return nil, errors.Errorf("fetching space: %w", err)
	}

	fromInput := make(map[network.SpaceName]struct{}, len(inputs))
	for _, space := range inputs {
		fromInput[space] = struct{}{}
	}

	// remove the empty space, representing a binding to the application's default
	// space.
	delete(fromInput, "")

	namesToUUID := make(map[network.SpaceName]string, len(spaces))
	// remove expected spaces from DB.
	for _, space := range spaces {
		delete(fromInput, network.SpaceName(space.Name))
		namesToUUID[network.SpaceName(space.Name)] = space.UUID
	}
	if len(fromInput) > 0 {
		var missingSpaces []network.SpaceName
		for space := range fromInput {
			missingSpaces = append(missingSpaces, space)
		}
		return nil, errors.
			Errorf("space(s) %q not found", missingSpaces).
			Add(applicationerrors.SpaceNotFound)
	}

	return namesToUUID, nil
}

// checkEndpointBindingName validates that the binding names in the input are
// included in the charm relations or extra bindings.
// It ensures no unexpected or unknown bindings exist and returns
// an error if unmatched bindings are found.
func (st *State) checkEndpointBindingName(
	charmRelations []charmRelationName,
	charmExtraBinding []charmExtraBinding,
	bindings map[string]network.SpaceName,
) error {
	fromInput := set.NewStrings(slices.Collect(maps.Keys(bindings))...)

	// remove the eventual empty relation for "default" space.
	fromInput.Remove("")

	// remove expected relation from DB.
	for _, relation := range charmRelations {
		fromInput.Remove(relation.Name)
	}
	for _, binding := range charmExtraBinding {
		fromInput.Remove(binding.Name)
	}
	if fromInput.Size() > 0 {
		return errors.
			Errorf("charm relation(s) or extra binding %q not found", strings.Join(fromInput.Values(), ",")).
			Add(applicationerrors.CharmRelationNotFound)
	}

	return nil
}

// updateDefaultSpace updates the default space binding for an application in the database.
// It uses the provided transaction to set the default space based on the binding map.
// If no default space is specified in the bindings map, the operation is a no-op.
func (st *State) updateDefaultSpace(ctx context.Context, tx *sqlair.TX, appID string, bindings map[string]network.SpaceName) error {
	defaultSpace, ok := bindings[""]
	if !ok {
		// No default space, noop.
		return nil
	}
	app := setDefaultSpace{UUID: appID, Space: defaultSpace.String()}
	updateDefaultSpaceStmt, err := st.Prepare(`
UPDATE application 
SET space_uuid = (
    SELECT uuid
    FROM space
    WHERE name = $setDefaultSpace.space    
)
WHERE uuid =  $setDefaultSpace.uuid`, app)
	if err != nil {
		return errors.Errorf("preparing update default space: %w", err)
	}
	return tx.Query(ctx, updateDefaultSpaceStmt, app).Run()
}

// getEndpointBindings gets a map of endpoint names to space UUIDs. This
// includes the application endpoints, and the application extra endpoints. An
// endpoint name of "" is used to record the default application space.
func (st *State) getEndpointBindings(ctx context.Context, tx *sqlair.TX, appUUID coreapplication.ID) (map[string]network.SpaceUUID, error) {
	// Query application endpoints.
	id := applicationID{ID: appUUID}
	endpointStmt, err := st.Prepare(`
SELECT (ae.space_uuid, cr.name) AS (&endpointBinding.*)
FROM   application_endpoint ae
JOIN   charm_relation cr ON cr.uuid = ae.charm_relation_uuid
WHERE  ae.application_uuid = $applicationID.uuid
`, endpointBinding{}, id)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Query application extra endpoints.
	extraEndpointStmt, err := st.Prepare(`
SELECT (aee.space_uuid, ceb.name) AS (&endpointBinding.*)
FROM   application_extra_endpoint aee
JOIN   charm_extra_binding ceb ON ceb.uuid = aee.charm_extra_binding_uuid
WHERE  aee.application_uuid = $applicationID.uuid
`, endpointBinding{}, id)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Query default endpoint for application.
	defaultSpaceUUID := spaceUUID{}
	defaultEndpointStmt, err := st.Prepare(`
SELECT space_uuid AS &spaceUUID.uuid
FROM   application 
WHERE  uuid = $applicationID.uuid
`, defaultSpaceUUID, id)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Get application endpoints.
	var dbEndpoints []endpointBinding
	err = tx.Query(ctx, endpointStmt, id).GetAll(&dbEndpoints)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("getting application endpoints: %w", err)
	}

	// Get application extra endpoints.
	var dbExtraEndpoints []endpointBinding
	err = tx.Query(ctx, extraEndpointStmt, id).GetAll(&dbExtraEndpoints)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("getting application endpoints: %w", err)
	}

	// Get application default endpoint.
	err = tx.Query(ctx, defaultEndpointStmt, id).Get(&defaultSpaceUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, applicationerrors.ApplicationNotFound
	} else if err != nil {
		return nil, errors.Errorf("getting application endpoints: %w", err)
	}

	endpoints := make(map[string]network.SpaceUUID, len(dbEndpoints)+len(dbExtraEndpoints)+1)
	for _, e := range dbEndpoints {
		if e.SpaceUUID.Valid {
			endpoints[e.EndpointName] = e.SpaceUUID.V
		} else {
			endpoints[e.EndpointName] = network.SpaceUUID(defaultSpaceUUID.UUID)
		}
	}
	for _, e := range dbExtraEndpoints {
		if e.SpaceUUID.Valid {
			endpoints[e.EndpointName] = e.SpaceUUID.V
		} else {
			endpoints[e.EndpointName] = network.SpaceUUID(defaultSpaceUUID.UUID)
		}
	}
	endpoints[""] = network.SpaceUUID(defaultSpaceUUID.UUID)

	return endpoints, nil
}

// bindingTable is used to indicate whether an endpoint name is a relation
// name or an extra endpoint name as defined in the charm metadata. It
// identifies which table should be used when updating the endpoint bindings
// by name.
type bindingTable string

const (
	BindingRelation bindingTable = "relation"
	BindingExtra    bindingTable = "extra"
)

// getBindingTableTypes validates that the binding names provided exist in
// the application's charm as relations or extra bindings. Returns a
// map of binding names to their table types.
func (st *State) getBindingTableTypes(ctx context.Context, tx *sqlair.TX, appID string, names []string) (map[string]bindingToTable, error) {
	// an empty string indicates the application's
	// default space should be updated, no relation
	// or extra binding to find.
	bindingNames := set.NewStrings(names...)
	bindingNames.Remove("")

	if bindingNames.Size() == 0 {
		return map[string]bindingToTable{}, nil
	}

	query := `
WITH
charm_bindings AS (
    SELECT cr.name, cr.uuid, a.uuid AS application_uuid, 'relation' AS binding_type
    FROM   charm_relation AS cr
    JOIN   charm AS c ON cr.charm_uuid = c.uuid
    JOIN   application AS a ON c.uuid = a.charm_uuid

    UNION ALL

    SELECT ceb.name, ceb.uuid, a.uuid AS application_uuid, 'extra' AS binding_type
    FROM   charm_extra_binding AS ceb
    JOIN   charm AS c ON ceb.charm_uuid = c.uuid
    JOIN   application AS a ON c.uuid = a.charm_uuid
)
SELECT &bindingToTable.*
FROM   charm_bindings
WHERE  name IN ($endpointNames[:])
AND    application_uuid = $applicationID.uuid
`
	relationEndpointStmt, err := st.Prepare(query, bindingToTable{}, applicationID{}, endpointNames{})
	if err != nil {
		return nil, errors.Errorf("preparing charm endpoint count query: %w", err)
	}

	applicationID := applicationID{ID: coreapplication.ID(appID)}
	eps := endpointNames(names)

	var result []bindingToTable
	err = tx.Query(ctx, relationEndpointStmt, applicationID, eps).GetAll(&result)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("checking if endpoints %+v exist: %w", names, err)
	}

	if len(result) != len(bindingNames) {
		return nil, errors.Errorf("one or more of the provided endpoints %q do not exist",
			strings.Join(bindingNames.SortedValues(), ", ")).
			Add(applicationerrors.EndpointNotFound)
	}
	return transform.SliceToMap(result, func(in bindingToTable) (string, bindingToTable) {
		return in.Name, in
	}), nil
}

// getApplicationEndpointSpaceUUIDs verifies that all provided space names
// exist in the database and returns a map of the requested space names to
// their UUIDs. If the input contains an empty string, the uuid of the
// application's default space is returned.
func (st *State) getApplicationEndpointSpaceUUIDs(
	ctx context.Context,
	tx *sqlair.TX,
	appID string,
	inputs []string,
) (map[string]string, error) {
	spaceNames := set.NewStrings(inputs...)

	type spaceInput []string
	fetchStmt, err := st.Prepare(
		`
WITH
app_default_space AS (
    SELECT s.name, s.uuid, a.uuid AS app_uuid
    FROM   space AS s
    JOIN   application AS a ON s.uuid = a.space_uuid
)
SELECT    (s.uuid, s.name) AS (&spaceWithAppDefault.*),
          ads.uuid AS &spaceWithAppDefault.app_uuid
FROM      space AS s
LEFT JOIN app_default_space AS ads ON s.uuid = ads.uuid
WHERE     s.name IN ($spaceInput[:])
OR        ads.app_uuid = $dbUUID.uuid
`, spaceWithAppDefault{}, spaceInput{}, dbUUID{})
	if err != nil {
		return nil, errors.Errorf("preparing fetch space: %w", err)
	}

	var (
		spaces []spaceWithAppDefault
	)
	err = tx.Query(ctx, fetchStmt, spaceInput(spaceNames.Values()), dbUUID{UUID: appID}).GetAll(&spaces)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, networkerrors.SpaceNotFound
	} else if err != nil {
		return nil, errors.Errorf("fetching space: %w", err)
	}

	result := transform.SliceToMap(spaces, func(s spaceWithAppDefault) (string, string) {
		name := s.Name
		if s.AppUUID != "" {
			name = ""
		}
		return name, s.UUID
	})

	// Results always contain the application's default space.
	// Remove from results if not asked for.
	if !spaceNames.Contains("") {
		delete(result, "")
	}
	if len(result) == spaceNames.Size() {
		return result, nil
	}

	// Find which spaces are missing for error message
	for name := range result {
		spaceNames.Remove(name)
	}
	return nil, errors.
		Errorf("space(s) %q not found", strings.Join(spaceNames.Values(), ",")).
		Add(networkerrors.SpaceNotFound)
}
