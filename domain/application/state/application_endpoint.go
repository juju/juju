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

// validateEndpointBindingsForApplication validates the provided endpoint bindings
// can be applied to the specified application.
// This checks that:
//   - we make sure that the endpoint names in the binding map are all present
//     in the provided charm metadata and that the space IDs actually exist.
//   - we check that all existing units for the application are executing on
//     machines that have an address in each space we are binding to. This
//     check can be circumvented by setting the force argument to true.
//
// It returns as error satisfying
//   - [applicationerrors.EndpointNotFound] if the endpoints changing do not exist
//     for the application's charm
//   - [networkerrors.SpaceNotFound] if the spaces provided do not exist, or the
//     application's units do not have addresses in the space.
func (st *State) validateEndpointBindingsForApplication(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	bindings map[string]network.SpaceName,
	force bool,
) error {
	if err := st.checkApplicationAlive(ctx, tx, appID); err != nil {
		return errors.Capture(err)
	}

	// Validate that provided Space names exist.
	spaceNamesToUUID, err := st.getSpaceUUIDs(ctx, tx, appID, slices.Collect(maps.Values(bindings)))
	if err != nil {
		return errors.Errorf("validating spaces exist: %w", err)
	}

	// Validate that all binding names exist in the charm as relation
	// endpoints or extra bindings. Not including the default binding for
	// the application as that always exists.
	bindingsToFind := make([]string, 0)
	for k := range bindings {
		if k == "" {
			continue
		}
		bindingsToFind = append(bindingsToFind, k)
	}
	if err := st.checkBindingsExist(ctx, tx, appID, bindingsToFind); err != nil {
		return errors.Errorf("validating endpoints exist: %w", err)
	}

	// If forced, no need to verify unit ip addresses in
	// provided spaces.
	if force {
		// Do not validate that all existing units have an address in
		// each space being bound to.
		return nil
	}

	// Get space UUIDs for all units of the application.
	unitSpaceUUIDs, err := st.getUnitSpaceUUIDsByApplication(ctx, tx, appID)
	if err != nil {
		return errors.Errorf("gathering spaces for units: %w", err)
	}

	// Check all units have addresses on the input spaces.
	inputSpaceUUIDsSet := set.NewStrings(slices.Collect(maps.Values(spaceNamesToUUID))...)
	for unitName, spaceUUIDs := range unitSpaceUUIDs {
		// Are all input space UUIDs contained in the set of unit space UUIDs?
		unitSpaceUUIDsSet := set.NewStrings(spaceUUIDs...)
		if !inputSpaceUUIDsSet.Difference(unitSpaceUUIDsSet).IsEmpty() {
			return errors.Errorf("unit %q is not in every space: %s", unitName, unitSpaceUUIDsSet.Values())
		}
	}
	return nil
}

// MergeApplicationEndpointBindings merges the provided bindings into the bindings
// for the specified application.
// The following errors may be returned:
//   - [applicationerrors.ApplicationNotFound] if the application does not exist
//   - [applicationerrors.EndpointNotFound] if the endpoints changing do not exist
//     for the application's charm
//   - [networkerrors.SpaceNotFound] if the spaces provided do not exist, or the
//     application's units do not have addresses in the space.
func (st *State) MergeApplicationEndpointBindings(
	ctx context.Context,
	appID coreapplication.ID,
	bindings map[string]network.SpaceName,
	force bool,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.validateEndpointBindingsForApplication(ctx, tx, appID, bindings, force); err != nil {
			return errors.Errorf("validating endpoint bindings: %w", err)
		}

		err := st.updateDefaultSpace(ctx, tx, appID, bindings)
		if err != nil {
			return errors.Errorf("updating default space: %w", err)
		}

		err = st.updateApplicationEndpointBindings(ctx, tx, updateApplicationEndpointsParams{
			appID:    appID,
			bindings: bindings,
		})
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
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

// updateApplicationEndpointsParams contains parameters required to insert
// application endpoints into the database.
type updateApplicationEndpointsParams struct {
	appID coreapplication.ID

	// EndpointBindings is a map to bind application endpoint by name to a
	// specific space. The default space is referenced by an empty key, if any.
	bindings map[string]network.SpaceName
}

func (st *State) updateApplicationEndpointBindings(ctx context.Context, tx *sqlair.TX, params updateApplicationEndpointsParams) error {
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

	// Update endpoints.
	for _, relation := range relations {
		if spaceName, ok := params.bindings[relation.Name]; ok {
			if err := st.updateApplicationEndpointBinding(ctx, tx, params.appID, spaceNamesToUUID, relation, spaceName); err != nil {
				return errors.Errorf("updating application endpoint: %w", err)
			}
		}
	}

	// Update extra binding
	for _, binding := range extrabindings {
		if spaceName, ok := params.bindings[binding.Name]; ok {
			if err := st.updateApplicationExtraBinding(ctx, tx, params.appID, spaceNamesToUUID, binding, spaceName); err != nil {
				return errors.Errorf("updating application extra bindings: %w", err)
			}
		}
	}

	return nil
}

func (st *State) updateApplicationEndpointBinding(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	spaceNamesToUUID map[network.SpaceName]string,
	relation charmRelationName,
	spaceName network.SpaceName,
) error {
	updateApplicationEndpointStmt, err := st.Prepare(`
UPDATE application_endpoint
SET space_uuid = $updateApplicationEndpointBinding.space_uuid
WHERE application_uuid = $updateApplicationEndpointBinding.application_uuid
AND charm_relation_uuid = $updateApplicationEndpointBinding.charm_relation_uuid
`, updateApplicationEndpointBinding{})
	if err != nil {
		return errors.Errorf("preparing update application endpoint: %w", err)
	}

	space := sql.Null[string]{}
	if spaceName != "" {
		uuid, ok := spaceNamesToUUID[spaceName]
		if !ok {
			return errors.Errorf("space %q not found", spaceName)
		}
		space = sql.Null[string]{
			V:     uuid,
			Valid: true,
		}
	}

	return tx.Query(ctx, updateApplicationEndpointStmt, updateApplicationEndpointBinding{
		ApplicationID: appID,
		RelationUUID:  relation.UUID,
		Space:         space,
	}).Run()
}

func (st *State) updateApplicationExtraBinding(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	spaceNamesToUUID map[network.SpaceName]string,
	binding charmExtraBinding,
	spaceName network.SpaceName,
) error {
	updateApplicationExtraEndpointStmt, err := st.Prepare(`
UPDATE application_extra_endpoint
SET space_uuid = $updateApplicationExtraEndpointBinding.space_uuid
WHERE application_uuid = $updateApplicationExtraEndpointBinding.application_uuid
AND charm_extra_binding_uuid = $updateApplicationExtraEndpointBinding.charm_extra_binding_uuid
`, updateApplicationExtraEndpointBinding{})
	if err != nil {
		return errors.Errorf("preparing update application extra endpoint: %w", err)
	}

	space := sql.Null[string]{}
	if spaceName != "" {
		uuid, ok := spaceNamesToUUID[spaceName]
		if !ok {
			return errors.Errorf("space %q not found", spaceName)
		}
		space = sql.Null[string]{
			V:     uuid,
			Valid: true,
		}
	}

	return tx.Query(ctx, updateApplicationExtraEndpointStmt, updateApplicationExtraEndpointBinding{
		ApplicationID: appID,
		RelationUUID:  binding.UUID,
		Space:         space,
	}).Run()
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
// database and returns a map of all space names to their UUIDs.
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
	namesToInclude := set.NewStrings()
	for _, in := range inputs {
		namesToInclude.Add(in.String())
	}

	// remove expected spaces from DB.
	for _, space := range spaces {
		delete(fromInput, network.SpaceName(space.Name))
		// only return space uuids for requested space names
		if namesToInclude.Contains(space.Name) {
			namesToUUID[network.SpaceName(space.Name)] = space.UUID
		}
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

func (st *State) getApplicationDefaultSpaceName(ctx context.Context, tx *sqlair.TX, appID coreapplication.ID) (string, error) {
	defaultSpaceStmt, err := st.Prepare(`
SELECT s.name AS &spaceName.*
FROM   space AS s
JOIN   application AS a ON s.uuid = a.space_uuid
WHERE  a.uuid = $dbUUID.uuid
`, spaceName{}, dbUUID{})
	if err != nil {
		return "", errors.Capture(err)
	}

	var name spaceName
	err = tx.Query(ctx, defaultSpaceStmt, dbUUID{UUID: appID.String()}).Get(&name)
	if err != nil {
		return "", errors.Errorf("fetching application %q default space: %w", appID, err)
	}

	return name.Name, nil
}

// ensureSpaceNamesSet returns a set of unique strings. If an empty string
// is provided as input, it is replaced with the default space name for
// the application.
func (st *State) ensureSpaceNamesSet(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	inputs []network.SpaceName,
) (set.Strings, error) {
	spaceNames := set.NewStrings()
	var found bool
	for _, in := range inputs {
		if in != "" {
			spaceNames.Add(in.String())
			continue
		}
		// Only get the application's default space once.
		if found {
			continue
		}
		// get default space for app
		defaultSpaceName, err := st.getApplicationDefaultSpaceName(ctx, tx, appID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		spaceNames.Add(defaultSpaceName)
		found = true
	}
	return spaceNames, nil
}

// getSpaceUUIDs verifies that all provided network space names exist in the
// database and returns a map of the requested space names to their UUIDs.
func (st *State) getSpaceUUIDs(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	inputs []network.SpaceName,
) (map[network.SpaceName]string, error) {
	spaceNames, err := st.ensureSpaceNamesSet(ctx, tx, appID, inputs)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type spaceInput []string
	fetchStmt, err := st.Prepare(`
SELECT &space.*
FROM   space
WHERE  name IN ($spaceInput[:])`, space{}, spaceInput{})
	if err != nil {
		return nil, errors.Errorf("preparing fetch space: %w", err)
	}

	var spaces []space
	err = tx.Query(ctx, fetchStmt, spaceInput(spaceNames.Values())).GetAll(&spaces)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, networkerrors.SpaceNotFound
	} else if err != nil {
		return nil, errors.Errorf("fetching space: %w", err)
	}

	result := transform.SliceToMap(spaces, func(s space) (network.SpaceName, string) {
		return network.SpaceName(s.Name), s.UUID
	})
	if len(result) == spaceNames.Size() {
		return result, nil
	}

	// Find which spaces are missing for error message
	for name := range result {
		spaceNames.Remove(name.String())
	}
	return nil, errors.
		Errorf("space(s) %q not found", strings.Join(spaceNames.Values(), ",")).
		Add(networkerrors.SpaceNotFound)
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
func (st *State) updateDefaultSpace(ctx context.Context, tx *sqlair.TX, appID coreapplication.ID, bindings map[string]network.SpaceName) error {
	defaultSpace, ok := bindings[""]
	if !ok {
		// No default space, noop.
		return nil
	}
	app := setDefaultSpace{UUID: appID, Space: defaultSpace}
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

// checkBindingsExist checks that the binding names provided exist in
// the application's charm as relations or extra bindings.
func (st *State) checkBindingsExist(ctx context.Context, tx *sqlair.TX, appID coreapplication.ID, names []string) error {
	query := `
SELECT COUNT(*) AS &countResult.count
FROM (
        SELECT cr.name
        FROM   charm_relation AS cr
        JOIN   charm AS c ON cr.charm_uuid = c.uuid
        JOIN   application AS a ON c.uuid = a.charm_uuid
        WHERE  cr.name IN ($endpointNames[:])
        AND    a.uuid = $applicationID.uuid

        UNION

        SELECT ceb.name
        FROM   charm_extra_binding AS ceb
        JOIN   charm AS c ON ceb.charm_uuid = c.uuid
        JOIN   application AS a ON c.uuid = a.charm_uuid
        WHERE  ceb.name IN ($endpointNames[:])
        AND    a.uuid = $applicationID.uuid
    )
`
	relationEndpointStmt, err := st.Prepare(query, countResult{}, applicationID{}, endpointNames{})
	if err != nil {
		return errors.Errorf("preparing charm endpoint count query: %w", err)
	}

	applicationID := applicationID{ID: appID}
	eps := endpointNames(names)

	var count countResult
	err = tx.Query(ctx, relationEndpointStmt, applicationID, eps).Get(&count)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking if endpoints %+v exist: %w", names, err)
	}

	if count.Count != len(names) {
		return errors.Errorf("one or more of the provided endpoints %+v do not exist", names).
			Add(applicationerrors.EndpointNotFound)
	}
	return nil
}

// getUnitSpaceUUIDsByApplication returns a map of slice of space UUIDs the
// unit is in, keyed by unit name.
func (st *State) getUnitSpaceUUIDsByApplication(ctx context.Context, tx *sqlair.TX, appID coreapplication.ID) (map[string][]string, error) {
	app := applicationID{ID: appID}

	unitNetNodeStmt, err := st.Prepare(`
SELECT sn.space_uuid AS &space.uuid,
       u.name AS &space.name
FROM   unit AS u
JOIN   net_node AS nn ON u.net_node_uuid = nn.uuid
JOIN   link_layer_device AS lld ON nn.uuid = lld.net_node_uuid
JOIN   ip_address AS ip ON lld.uuid = ip.device_uuid
JOIN   subnet AS sn ON ip.subnet_uuid = sn.uuid
WHERE  u.application_uuid = $applicationID.uuid
`, applicationID{}, space{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var spaces []space
	err = tx.Query(ctx, unitNetNodeStmt, app).GetAll(&spaces)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	} else if errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("application %s: %w", appID, networkerrors.SpaceNotFound)
	}

	return unitSpaceToMap(spaces), nil
}

func unitSpaceToMap(in []space) map[string][]string {
	out := make(map[string][]string)
	for _, sp := range in {
		spUUIDs, ok := out[sp.Name]
		if !ok {
			spUUIDs = make([]string, 0)
		}
		out[sp.Name] = append(spUUIDs, sp.UUID)
	}
	return out
}
