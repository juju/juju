// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalerrors "github.com/juju/juju/internal/errors"
)

// insertApplicationEndpointsParams contains parameters required to insert
// application endpoints into the database.
type insertApplicationEndpointsParams struct {
	appID     coreapplication.ID
	charmUUID corecharm.ID

	// EndpointBindings is a map to bind application endpoint by name to a
	// specific space. The default space is referenced by an empty key, if any.
	bindings map[string]network.SpaceName
}

// insertApplicationEndpoints inserts database records for an
// application's endpoints (`application_endpoint` and
// `application_extra_endpoint`) and handle space configuration on both
// endpoint and application level.
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
func (st *State) insertApplicationEndpoints(ctx context.Context, tx *sqlair.TX, params insertApplicationEndpointsParams) error {
	charmUUID := charmID{UUID: params.charmUUID}

	// Get charm relation.
	relations, err := st.getCharmRelationNames(ctx, tx, charmUUID)
	if err != nil {
		return internalerrors.Errorf("getting charm relation names: %w", err)
	}

	// Get extra bindings
	extrabindings, err := st.getCharmExtraBindings(ctx, tx, charmUUID)
	if err != nil {
		return internalerrors.Errorf("getting charm extra bindings: %w", err)
	}

	// Check that spaces are valid in binding.
	if err := st.checkSpaceNames(ctx, tx, slices.Collect(maps.Values(params.bindings))); err != nil {
		return internalerrors.Errorf("checking space names: %w", err)
	}
	// Check that binding are linked to valid endpoint (either from a charm relation
	// or an extra binding.
	if err := st.checkEndpointBindingName(relations, extrabindings, params.bindings); err != nil {
		return internalerrors.Errorf("checking charm relation: %w", err)
	}

	// Update default space.
	if err := st.updateDefaultSpace(ctx, tx, params.appID, params.bindings); err != nil {
		return internalerrors.Errorf("updating default space: %w", err)
	}

	// Insert endpoints.
	for _, relation := range relations {
		if err := st.insertApplicationEndpoint(ctx, tx, params.appID, relation, params.bindings); err != nil {
			return internalerrors.Errorf("inserting application endpoint: %w", err)
		}
	}

	// Insert extra binding
	for _, binding := range extrabindings {
		if err := st.insertApplicationExtraBinding(ctx, tx, params.appID, binding, params.bindings); err != nil {
			return internalerrors.Errorf("inserting application endpoint: %w", err)
		}
	}

	return nil
}

// insertApplicationEndpoint inserts an application endpoint into the database,
// associating it with a relation and space.
func (st *State) insertApplicationEndpoint(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	relation charmRelationName,
	bindings map[string]network.SpaceName,
) error {
	insertApplicationEndpointStmt, err := st.Prepare(`
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid, space_uuid)
SELECT $setApplicationEndpoint.uuid,
       $setApplicationEndpoint.application_uuid,
       $setApplicationEndpoint.charm_relation_uuid,
       sp.uuid
FROM (
    SELECT uuid FROM space
	WHERE name = $setApplicationEndpoint.space
	UNION ALL -- This allows to insert null space_uuid if a null space is provided.
	SELECT NULL AS uuid
) AS sp
LIMIT 1
`, setApplicationEndpoint{})
	if err != nil {
		return internalerrors.Errorf("preparing insert application endpoint: %w", err)
	}

	// Generate UUID
	uuid, err := corerelation.NewEndpointUUID()
	if err != nil {
		return internalerrors.Capture(err)
	}

	space := bindings[relation.Name]
	nilEmpty := func(s network.SpaceName) *string {
		if s == "" {
			return nil
		}
		res := string(s)
		return &res
	}

	return tx.Query(ctx, insertApplicationEndpointStmt, setApplicationEndpoint{
		UUID:          uuid,
		ApplicationID: appID,
		RelationUUID:  relation.UUID,
		Space:         nilEmpty(space),
	}).Run()
}

// insertApplicationExtraBinding inserts a charm extra binding into the database,
// associating it with a relation and space.
func (st *State) insertApplicationExtraBinding(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	binding charmExtraBinding,
	bindings map[string]network.SpaceName,
) error {
	insertStmt, err := st.Prepare(`
INSERT INTO application_extra_endpoint (application_uuid, charm_extra_binding_uuid, space_uuid)
SELECT $setApplicationExtraEndpoint.application_uuid,
       $setApplicationExtraEndpoint.charm_extra_binding_uuid,
       sp.uuid
FROM (
    SELECT uuid FROM space
	WHERE name = $setApplicationExtraEndpoint.space
	UNION ALL -- This allows to insert null space_uuid if a null space is provided.
	SELECT NULL AS uuid
) AS sp
LIMIT 1
`, setApplicationExtraEndpoint{})
	if err != nil {
		return internalerrors.Errorf("preparing insert application extra endpoint: %w", err)
	}

	space := bindings[binding.Name]
	nilEmpty := func(s network.SpaceName) *string {
		if s == "" {
			return nil
		}
		res := string(s)
		return &res
	}

	return tx.Query(ctx, insertStmt, setApplicationExtraEndpoint{
		ApplicationID: appID,
		RelationUUID:  binding.UUID,
		Space:         nilEmpty(space),
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
		return nil, internalerrors.Errorf("preparing fetch charm relation: %w", err)
	}
	var relations []charmRelationName
	if err := tx.Query(ctx, fetchCharmRelationStmt, charmUUID).GetAll(&relations); err != nil && !errors.Is(err,
		sqlair.ErrNoRows) {
		return nil, internalerrors.Errorf("fetching charm relation: %w", err)
	}
	return relations, nil
}

// checkSpaceNames verifies that all provided network space names exist in the
// database and returns an error if any do not.
func (st *State) checkSpaceNames(ctx context.Context, tx *sqlair.TX, inputs []network.SpaceName) error {
	fetchStmt, err := st.Prepare(`
SELECT &spaceName.name
FROM space`, spaceName{})
	if err != nil {
		return internalerrors.Errorf("preparing fetch space: %w", err)
	}
	var spaces []spaceName
	if err := tx.Query(ctx, fetchStmt).GetAll(&spaces); err != nil {
		return internalerrors.Errorf("fetching space: %w", err)
	}
	fromInput := set.NewStrings()
	for _, space := range inputs {
		fromInput.Add(string(space))
	}

	// remove expected spaces from DB.
	for _, space := range spaces {
		fromInput.Remove(space.Name)
	}
	if fromInput.Size() > 0 {
		return internalerrors.
			Errorf("space(s) %q not found", strings.Join(fromInput.Values(), ",")).
			Add(applicationerrors.SpaceNotFound)
	}

	return nil
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
		return internalerrors.
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
		return internalerrors.Errorf("preparing update default space: %w", err)
	}
	return tx.Query(ctx, updateDefaultSpaceStmt, app).Run()
}

// getEndpointBindings gets a map of endpoint names to space UUIDs.
func (st *State) getEndpointBindings(ctx context.Context, tx *sqlair.TX, appUUID coreapplication.ID) (map[string]string, error) {
	// Get application endpoints.
	id := applicationID{ID: appUUID}
	endpointStmt, err := st.Prepare(`
SELECT (ae.space_uuid, cr.name) AS (&getApplicationEndpoint.*)
FROM   application_endpoint ae
JOIN   charm_relation cr ON cr.uuid = ae.charm_relation_uuid
WHERE  ae.application_uuid = $applicationID.uuid
`, getApplicationEndpoint{}, id)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	var dbEndpoints []getApplicationEndpoint
	err = tx.Query(ctx, endpointStmt, id).GetAll(&dbEndpoints)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, internalerrors.Errorf("getting application endpoints: %w", err)
	}

	endpoints := make(map[string]string, len(dbEndpoints))
	for _, e := range dbEndpoints {
		endpoints[e.EndpointName] = e.SpaceUUID
	}

	// Get default endpoint for application.
	defaultSpaceUUID := spaceUUID{}
	defaultEndpointStmt, err := st.Prepare(`
SELECT space_uuid AS &spaceUUID.uuid
FROM   application 
WHERE  uuid = $applicationID.uuid
`, defaultSpaceUUID, id)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	err = tx.Query(ctx, defaultEndpointStmt, id).Get(&defaultSpaceUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, applicationerrors.ApplicationNotFound
	} else if err != nil {
		return nil, internalerrors.Errorf("getting application endpoints: %w", err)
	}
	endpoints[""] = defaultSpaceUUID.UUID

	return endpoints, nil
}
