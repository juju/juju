// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/life"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// AddRemoteApplicationOfferer adds a new remote application offerer that
// is on the consumer side of a cross-model relation. This inserts a
// synthetic application and charm into the model to represent the remote
// application.
func (st *State) AddRemoteApplicationOfferer(
	ctx context.Context,
	applicationName string,
	args crossmodelrelation.AddRemoteApplicationOffererArgs,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	appUUID, err := coreapplication.NewID()
	if err != nil {
		return errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Insert the application, along with the associated charm.
		if err := st.insertApplication(ctx, tx, applicationName, appUUID, args); err != nil {
			return errors.Capture(err)
		}

		// Insert the remote application offerer record, this allows us to find
		// the synthetic application later.
		if err := st.insertRemoteApplicationOfferer(ctx, tx, appUUID, args); err != nil {
			return errors.Capture(err)
		}

		return nil
	}); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) insertApplication(
	ctx context.Context,
	tx *sqlair.TX,
	name string,
	appUUID coreapplication.ID,
	args crossmodelrelation.AddRemoteApplicationOffererArgs,
) error {
	charmID, err := corecharm.NewID()
	if err != nil {
		return errors.Capture(err)
	}

	appDetails := applicationDetails{
		UUID:      appUUID.String(),
		Name:      name,
		CharmUUID: charmID.String(),
		LifeID:    life.Alive,

		// SpaceUUID here is to prevent the FK violation, but we push it
		// into the default alpha space. We'll need to ensure that this
		// application is not used in any network operations.
		SpaceUUID: network.AlphaSpaceId.String(),
	}

	createApplication := `INSERT INTO application (*) VALUES ($applicationDetails.*)`
	createApplicationStmt, err := st.Prepare(createApplication, appDetails)
	if err != nil {
		return errors.Capture(err)
	}

	// Check if the application already exists.
	if err := st.checkApplicationNameAvailable(ctx, tx, name); err != nil {
		return errors.Errorf("checking if application %q exists: %w", name, err)
	}

	if err := st.addCharm(ctx, tx, charmID, args.Charm); err != nil {
		return errors.Errorf("setting charm: %w", err)
	}

	// If the application doesn't exist, create it.
	if err := tx.Query(ctx, createApplicationStmt, appDetails).Run(); err != nil {
		return errors.Errorf("inserting row for application %q: %w", name, err)
	}

	return nil
}

func (st *State) insertRemoteApplicationOfferer(
	ctx context.Context,
	tx *sqlair.TX,
	applicationUUID coreapplication.ID,
	args crossmodelrelation.AddRemoteApplicationOffererArgs,
) error {
	return nil
}

// checkApplicationNameAvailable checks if the application name is available.
// If the application name is available, nil is returned. If the application
// name is not available, [applicationerrors.ApplicationAlreadyExists] is
// returned.
func (st *State) checkApplicationNameAvailable(ctx context.Context, tx *sqlair.TX, name string) error {
	app := applicationDetails{Name: name}

	var result countResult
	existsQueryStmt, err := st.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM application
WHERE name = $applicationDetails.name
`, app, result)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, existsQueryStmt, app).Get(&result); errors.Is(err, sqlair.ErrNoRows) {
		return nil
	} else if err != nil {
		return errors.Errorf("checking if application %q exists: %w", name, err)
	}
	if result.Count > 0 {
		return applicationerrors.ApplicationAlreadyExists
	}
	return nil
}

func (s *State) addCharm(ctx context.Context, tx *sqlair.TX, uuid corecharm.ID, ch charm.Charm) error {
	if err := s.addCharmState(ctx, tx, uuid, ch); err != nil {
		return errors.Capture(err)
	}

	if err := s.addCharmMetadata(ctx, tx, uuid, ch.Metadata); err != nil {
		return errors.Capture(err)
	}

	if err := s.addCharmRelations(ctx, tx, uuid, ch.Metadata); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (s *State) addCharmState(
	ctx context.Context,
	tx *sqlair.TX,
	id corecharm.ID,
	ch charm.Charm,
) error {
	sourceID, err := encodeCharmSource(ch.Source)
	if err != nil {
		return errors.Errorf("encoding charm source: %w", err)
	}

	chState := setCharmState{
		UUID:          id.String(),
		ReferenceName: ch.ReferenceName,
		SourceID:      sourceID,
	}

	charmQuery := `INSERT INTO charm (*) VALUES ($setCharmState.*);`
	charmStmt, err := s.Prepare(charmQuery, chState)
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, charmStmt, chState).Run(); err != nil {
		return errors.Errorf("inserting charm state: %w", err)
	}

	return nil
}

func (s *State) addCharmMetadata(
	ctx context.Context,
	tx *sqlair.TX,
	id corecharm.ID,
	metadata charm.Metadata,
) error {
	encodedMetadata, err := encodeMetadata(id, metadata)
	if err != nil {
		return errors.Errorf("encoding charm metadata: %w", err)
	}

	query := `INSERT INTO charm_metadata (*) VALUES ($setCharmMetadata.*);`
	stmt, err := s.Prepare(query, encodedMetadata)
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedMetadata).Run(); err != nil {
		return errors.Errorf("inserting charm metadata: %w", err)
	}

	return nil
}

func (s *State) addCharmRelations(ctx context.Context, tx *sqlair.TX, id corecharm.ID, metadata charm.Metadata) error {
	encodedRelations, err := encodeRelations(id, metadata)
	if err != nil {
		return errors.Errorf("encoding charm relations: %w", err)
	}

	// juju-info is a implicit endpoint that must exist for all charms.
	// Add it if the charm author has not.
	if !hasJujuInfoRelation(encodedRelations) {
		jujuInfoRelation, err := encodeJujuInfoRelation(id)
		if err != nil {
			return errors.Errorf("encoding juju-info relation: %w", err)
		}
		encodedRelations = append(encodedRelations, jujuInfoRelation)
	}

	// If there are no relations, we don't need to do anything.
	if len(encodedRelations) == 0 {
		return nil
	}

	query := `INSERT INTO charm_relation (*) VALUES ($setCharmRelation.*);`
	stmt, err := s.Prepare(query, setCharmRelation{})
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedRelations).Run(); internaldatabase.IsErrConstraintUnique(err) {
		return applicationerrors.CharmRelationNameConflict
	} else if err != nil {
		return errors.Errorf("inserting charm relations: %w", err)
	}

	return nil
}

func encodeCharmSource(source charm.CharmSource) (int, error) {
	switch source {
	case charm.CMRSource:
		return 2, nil
	default:
		return 0, errors.Errorf("unsupported source type: %s", source)
	}
}

func encodeMetadata(id corecharm.ID, metadata charm.Metadata) (setCharmMetadata, error) {
	return setCharmMetadata{
		CharmUUID:   id.String(),
		Name:        metadata.Name,
		Description: metadata.Description,
	}, nil
}

func encodeRelations(id corecharm.ID, metatadata charm.Metadata) ([]setCharmRelation, error) {
	var result []setCharmRelation
	for _, relation := range metatadata.Provides {
		encoded, err := encodeRelation(id, relation)
		if err != nil {
			return nil, errors.Errorf("cannot encode provides relation: %w", err)
		}
		result = append(result, encoded)
	}

	for _, relation := range metatadata.Requires {
		encoded, err := encodeRelation(id, relation)
		if err != nil {
			return nil, errors.Errorf("cannot encode requires relation: %w", err)
		}
		result = append(result, encoded)
	}

	for _, relation := range metatadata.Peers {
		encoded, err := encodeRelation(id, relation)
		if err != nil {
			return nil, errors.Errorf("cannot encode peers relation: %w", err)
		}
		result = append(result, encoded)
	}

	return result, nil
}

func encodeJujuInfoRelation(id corecharm.ID) (setCharmRelation, error) {
	return encodeRelation(id, charm.Relation{
		Name:      corerelation.JujuInfo,
		Role:      charm.RoleProvider,
		Interface: corerelation.JujuInfo,
		Scope:     charm.ScopeGlobal,
	})
}

func encodeRelation(id corecharm.ID, relation charm.Relation) (setCharmRelation, error) {
	relationUUID, err := internaluuid.NewUUID()
	if err != nil {
		return setCharmRelation{}, errors.Errorf("generating relation uuid")
	}

	roleID, err := encodeRelationRole(relation.Role)
	if err != nil {
		return setCharmRelation{}, errors.Errorf("encoding relation role %q: %w", relation.Role, err)
	}

	scopeID, err := encodeRelationScope(relation.Scope)
	if err != nil {
		return setCharmRelation{}, errors.Errorf("encoding relation scope %q: %w", relation.Scope, err)
	}

	return setCharmRelation{
		UUID:      relationUUID.String(),
		CharmUUID: id.String(),
		Name:      relation.Name,
		RoleID:    roleID,
		Interface: relation.Interface,
		Optional:  relation.Optional,
		Capacity:  relation.Limit,
		ScopeID:   scopeID,
	}, nil
}

func hasJujuInfoRelation(encodedRelations []setCharmRelation) bool {
	// Relation names must be unique.
	for _, encodedRelation := range encodedRelations {
		if encodedRelation.Name == corerelation.JujuInfo {
			return true
		}
	}
	return false
}

func encodeRelationRole(role charm.RelationRole) (int, error) {
	// This values are hardcoded to match the index relation role values in the
	// database.
	switch role {
	case charm.RoleProvider:
		return 0, nil
	case charm.RoleRequirer:
		return 1, nil
	case charm.RolePeer:
		return 2, nil
	default:
		return -1, errors.Errorf("unknown relation role %q", role)
	}
}

func encodeRelationScope(scope charm.RelationScope) (int, error) {
	// This values are hardcoded to match the index relation scope values in the
	// database.
	switch scope {
	case charm.ScopeGlobal:
		return 0, nil
	case charm.ScopeContainer:
		return 1, nil
	default:
		return -1, errors.Errorf("unknown relation scope %q", scope)
	}
}
