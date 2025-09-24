// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/life"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// AddRemoteApplicationOfferer adds a new synthetic application representing
// an offer from an external model, to this, the consuming model.
func (st *State) AddRemoteApplicationOfferer(
	ctx context.Context,
	applicationName string,
	args crossmodelrelation.AddRemoteApplicationOffererArgs,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check if the application already exists.
		if err := st.checkApplicationNameAvailable(ctx, tx, applicationName); err != nil {
			return errors.Errorf("checking if application %q exists: %w", applicationName, err)
		}
		// Check the offer doesn't already exist.
		if err := st.checkOfferDoesNotExist(ctx, tx, args.OfferUUID); err != nil {
			return errors.Capture(err)
		}

		// Insert the application, along with the associated charm.
		if err := st.insertApplication(ctx, tx, applicationName, args); err != nil {
			return errors.Capture(err)
		}

		// Insert the remote application offerer record, this allows us to find
		// the synthetic application later.
		if err := st.insertRemoteApplicationOfferer(ctx, tx, args); err != nil {
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
	args crossmodelrelation.AddRemoteApplicationOffererArgs,
) error {
	appDetails := applicationDetails{
		UUID:      args.ApplicationUUID,
		Name:      name,
		CharmUUID: args.CharmUUID,
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

	if err := st.addCharm(ctx, tx, args.CharmUUID, args.Charm); err != nil {
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
	args crossmodelrelation.AddRemoteApplicationOffererArgs,
) error {
	var offererControllerUUID sql.Null[string]
	if args.OffererControllerUUID != nil {
		offererControllerUUID = sql.Null[string]{
			V:     *args.OffererControllerUUID,
			Valid: true,
		}
	}

	version, err := st.nextRemoteApplicationOffererVersion(ctx, tx, args.OfferUUID)
	if err != nil {
		return errors.Capture(err)
	}

	remoteApp := remoteApplicationOfferer{
		UUID:                  args.RemoteApplicationUUID,
		LifeID:                life.Alive,
		ApplicationUUID:       args.ApplicationUUID,
		OfferUUID:             args.OfferUUID,
		Version:               version,
		OffererControllerUUID: offererControllerUUID,
		OffererModelUUID:      args.OffererModelUUID,
		Macaroon:              args.EncodedMacaroon,
	}

	insertRemoteApp := `INSERT INTO application_remote_offerer (*) VALUES ($remoteApplicationOfferer.*);`
	insertRemoteAppStmt, err := st.Prepare(insertRemoteApp, remoteApp)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, insertRemoteAppStmt, remoteApp).Run(); err != nil {
		return errors.Errorf("inserting remote application offerer record: %w", err)
	}

	return nil
}

func (st *State) nextRemoteApplicationOffererVersion(
	ctx context.Context,
	tx *sqlair.TX,
	offerUUID string,
) (uint64, error) {

	namespace := domainsequence.MakePrefixNamespace(crossmodelrelation.ApplicationRemoteOffererSequenceNamespace, offerUUID)
	nextVersion, err := sequencestate.NextValue(ctx, st, tx, namespace)
	if err != nil {
		return 0, errors.Errorf("getting next remote application offerer version: %w", err)
	}
	return nextVersion, nil
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

// checkOfferDoesNotExist checks if an offer with the given UUID already
// exists. It returns true if the offer exists, false if it does not.
func (st *State) checkOfferDoesNotExist(ctx context.Context, tx *sqlair.TX, offerUUID string) error {
	var result countResult

	uuid := uuid{UUID: offerUUID}
	existsQueryStmt, err := st.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM application_remote_offerer
WHERE offer_uuid = $uuid.uuid
`, uuid, result)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, existsQueryStmt, uuid).Get(&result); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking if offer %q exists: %w", offerUUID, err)
	} else if result.Count > 0 {
		return crossmodelrelationerrors.OfferAlreadyConsumed
	}

	return nil
}

func (s *State) addCharm(ctx context.Context, tx *sqlair.TX, uuid string, ch charm.Charm) error {
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
	uuid string,
	ch charm.Charm,
) error {
	sourceID, err := encodeCharmSource(ch.Source)
	if err != nil {
		return errors.Errorf("encoding charm source: %w", err)
	}

	chState := setCharmState{
		UUID:          uuid,
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
	uuid string,
	metadata charm.Metadata,
) error {
	encodedMetadata, err := encodeMetadata(uuid, metadata)
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

func (s *State) addCharmRelations(ctx context.Context, tx *sqlair.TX, uuid string, metadata charm.Metadata) error {
	encodedRelations, err := encodeRelations(uuid, metadata)
	if err != nil {
		return errors.Errorf("encoding charm relations: %w", err)
	}

	// juju-info is a implicit endpoint that must exist for all charms.
	// Add it if the charm author has not.
	if !hasJujuInfoRelation(encodedRelations) {
		jujuInfoRelation, err := encodeJujuInfoRelation(uuid)
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

func encodeMetadata(uuid string, metadata charm.Metadata) (setCharmMetadata, error) {
	return setCharmMetadata{
		CharmUUID:   uuid,
		Name:        metadata.Name,
		Description: metadata.Description,
	}, nil
}

func encodeRelations(uuid string, metadata charm.Metadata) ([]setCharmRelation, error) {
	var result []setCharmRelation
	for _, relation := range metadata.Provides {
		encoded, err := encodeRelation(uuid, relation)
		if err != nil {
			return nil, errors.Errorf("cannot encode provides relation: %w", err)
		}
		result = append(result, encoded)
	}

	for _, relation := range metadata.Requires {
		encoded, err := encodeRelation(uuid, relation)
		if err != nil {
			return nil, errors.Errorf("cannot encode requires relation: %w", err)
		}
		result = append(result, encoded)
	}

	return result, nil
}

func encodeJujuInfoRelation(uuid string) (setCharmRelation, error) {
	return encodeRelation(uuid, charm.Relation{
		Name:      corerelation.JujuInfo,
		Role:      charm.RoleProvider,
		Interface: corerelation.JujuInfo,
		Scope:     charm.ScopeGlobal,
	})
}

func encodeRelation(uuid string, relation charm.Relation) (setCharmRelation, error) {
	relationUUID, err := internaluuid.NewUUID()
	if err != nil {
		return setCharmRelation{}, errors.Errorf("generating relation uuid")
	}

	roleID, err := encodeRelationRole(relation.Role)
	if err != nil {
		return setCharmRelation{}, errors.Errorf("encoding relation role %q: %w", relation.Role, err)
	}

	return setCharmRelation{
		UUID:      relationUUID.String(),
		CharmUUID: uuid,
		Name:      relation.Name,
		RoleID:    roleID,
		Interface: relation.Interface,
		Capacity:  relation.Limit,

		// ScopeID is always hardcoded to 0 (global) for CMR relations. There
		// isn't a way to express any other type of scope in a CMR relation from
		// the API.
		ScopeID: 0,

		// Also there isn't a way to express optional relations, and thus
		// it is always false.
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
	default:
		return -1, errors.Errorf("role should not be a peer relation, got %q", role)
	}
}
