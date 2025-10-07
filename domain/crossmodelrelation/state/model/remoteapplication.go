// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/life"
	domainrelation "github.com/juju/juju/domain/relation"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/domain/status"
	internalcharm "github.com/juju/juju/internal/charm"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// NamespaceRemoteApplicationOfferers returns the remote application
// offerers.
func (st *State) NamespaceRemoteApplicationOfferers() string {
	return "application_remote_offerer"
}

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
		if err := st.checkApplicationRemoteOffererDoesNotExist(ctx, tx, args.OfferUUID); err != nil {
			return errors.Capture(err)
		}

		// Insert the application, along with the associated charm.
		if err := st.insertApplication(ctx, tx, applicationName, args.AddRemoteApplicationArgs); err != nil {
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

// AddRemoteApplicationConsumer adds a new synthetic application representing
// the remote relation on the consuming model, to this, the offering model.
// If no local application exists for which the given offer UUID was created,
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) AddRemoteApplicationConsumer(
	ctx context.Context,
	applicationName string,
	args crossmodelrelation.AddRemoteApplicationConsumerArgs,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		//Get the application UUID for which the offer UUID was created.
		localApplicationUUID, err := st.getApplicationUUIDByOfferUUID(ctx, tx, args.OfferUUID)
		if err != nil {
			return errors.Capture(err)
		}

		// Make sure we don't have the remote application consumer already
		// inserted in the db.
		if err := st.checkRemoteApplicationExists(ctx, tx, args.OfferUUID, args.RemoteApplicationUUID, args.RelationUUID); err != nil {
			return errors.Capture(err)
		}

		// Check if the application already exists.
		if err := st.checkApplicationNameAvailable(ctx, tx, applicationName); err != nil {
			return errors.Errorf("checking if application %q exists: %w", applicationName, err)
		}

		// Insert the application, along with the associated charm.
		if err := st.insertApplication(ctx, tx, applicationName, args.AddRemoteApplicationArgs); err != nil {
			return errors.Capture(err)
		}

		// Create the application remote relation for this consumer.
		applicationRemoteRelation, err := st.insertApplicationRemoteRelation(ctx, tx, args.RelationUUID)
		if err != nil {
			return errors.Capture(err)
		}

		// Create an offer connection for this consumer.
		offerConnectionUUID, err := st.insertOfferConnection(ctx, tx, args.OfferUUID, applicationRemoteRelation.RelationUUID)
		if err != nil {
			return errors.Capture(err)
		}

		// Insert the remote application consumer record, this allows us to find
		// the synthetic application later.
		if err := st.insertRemoteApplicationConsumer(ctx, tx, offerConnectionUUID, localApplicationUUID, args.ApplicationUUID, args.OfferUUID); err != nil {
			return errors.Capture(err)
		}

		return nil
	}); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// GetRemoteApplicationOfferers returns all the current non-dead remote
// application offerers in the local model.
func (st *State) GetRemoteApplicationOfferers(ctx context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `
SELECT  a.name AS &remoteApplicationOffererInfo.application_name,
        aro.life_id AS &remoteApplicationOffererInfo.life_id,
        aro.application_uuid AS &remoteApplicationOffererInfo.application_uuid,
        aro.offer_uuid AS &remoteApplicationOffererInfo.offer_uuid,
        aro.version AS &remoteApplicationOffererInfo.version,
        aro.offerer_model_uuid AS &remoteApplicationOffererInfo.offerer_model_uuid,
        aro.macaroon AS &remoteApplicationOffererInfo.macaroon
FROM    application_remote_offerer AS aro
JOIN    application AS a ON a.uuid = aro.application_uuid
WHERE   aro.life_id < 2;`
	queryStmt, err := st.Prepare(query, remoteApplicationOffererInfo{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var offerers []remoteApplicationOffererInfo
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, queryStmt).GetAll(&offerers); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	}); err != nil {
		return nil, errors.Errorf("querying remote application offerers: %w", err)
	}

	result := make([]crossmodelrelation.RemoteApplicationOfferer, len(offerers))
	for i, offerer := range offerers {
		macaroon, err := decodeMacaroon(offerer.Macaroon)
		if err != nil {
			return nil, errors.Errorf("decoding macaroon for remote application offerer %q: %w", offerer.ApplicationName, err)
		}

		result[i] = crossmodelrelation.RemoteApplicationOfferer{
			Life:             offerer.LifeID,
			ApplicationUUID:  offerer.ApplicationUUID,
			ApplicationName:  offerer.ApplicationName,
			OfferUUID:        offerer.OfferUUID,
			ConsumeVersion:   int(offerer.Version),
			OffererModelUUID: offerer.OffererModelUUID,
			Macaroon:         macaroon,
		}
	}

	return result, nil
}

// GetApplicationRemoteRelationByConsumerRelationUUID retrieves the application
// remote relation (relation_uuid and consumer_relation_uuid) for the supplied
// consumer relation UUID. Returns
// [crossmodelrelationerrors.RemoteRelationNotFound] if no such relation exists.
func (st *State) GetApplicationRemoteRelationByConsumerRelationUUID(
	ctx context.Context,
	consumerRelUUID string,
) (corerelation.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	ident := consumerRelationUUID{ConsumerRelationUUID: consumerRelUUID}

	stmt, err := st.Prepare(`
SELECT arr.relation_uuid AS &uuid.uuid
FROM   application_remote_relation AS arr
JOIN   relation AS r ON r.uuid = arr.relation_uuid
WHERE  arr.consumer_relation_uuid = $consumerRelationUUID.consumer_relation_uuid
`, uuid{}, ident)
	if err != nil {
		return "", errors.Capture(err)
	}

	var result uuid
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&result); errors.Is(err, sqlair.ErrNoRows) {
			return crossmodelrelationerrors.RemoteRelationNotFound
		} else if err != nil {
			return errors.Errorf("retrieving application remote relation %q: %w", consumerRelUUID, err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return corerelation.UUID(result.UUID), nil
}

func (st *State) insertApplication(
	ctx context.Context,
	tx *sqlair.TX,
	name string,
	args crossmodelrelation.AddRemoteApplicationArgs,
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

	// Insert the endpoint bindings for the application.
	if err := st.insertApplicationEndpointBindings(ctx, tx, args.ApplicationUUID, args.CharmUUID); err != nil {
		return errors.Errorf("inserting application endpoint bindings: %w", err)
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

	// Insert the status for the remote application offerer.
	statusInfo := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusUnknown,
		Message: "waiting for first status update",
		Since:   ptr(st.clock.Now().UTC()),
	}
	if err := st.insertRemoteApplicationOffererStatus(ctx, tx, args.RemoteApplicationUUID, statusInfo); err != nil {
		return errors.Errorf("inserting remote application offerer status: %w", err)
	}

	return nil
}

func (st *State) insertRemoteApplicationConsumer(
	ctx context.Context,
	tx *sqlair.TX,
	offerConnectionUUID string,
	localApplicationUUID string,
	consumerApplicationUUID string,
	offerUUID string,
) error {

	// Insert the remote application consumer record, this allows us to find
	// the synthetic application later.
	version, err := st.nextRemoteApplicationConsumerVersion(ctx, tx, offerUUID)
	if err != nil {
		return errors.Capture(err)
	}

	remoteAppConsumerUUID, err := internaluuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}
	remoteApp := remoteApplicationConsumer{
		UUID:                    remoteAppConsumerUUID.String(),
		OffererApplicationUUID:  localApplicationUUID,
		ConsumerApplicationUUID: consumerApplicationUUID,
		OfferConnectionUUID:     offerConnectionUUID,
		Version:                 version,
		LifeID:                  life.Alive,
	}

	insertRemoteApp := `
INSERT INTO application_remote_consumer (*) 
VALUES ($remoteApplicationConsumer.*);`
	insertRemoteAppStmt, err := st.Prepare(insertRemoteApp, remoteApp)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, insertRemoteAppStmt, remoteApp).Run(); err != nil {
		return errors.Errorf("inserting remote application consumer record: %w", err)
	}

	return nil
}

func (st *State) insertApplicationRemoteRelation(
	ctx context.Context,
	tx *sqlair.TX,
	consumerRelationUUID string,
) (applicationRemoteRelation, error) {
	// Create a relation UUID for the synthetic relation in the offerer model.
	relationUUID, err := internaluuid.NewUUID()
	if err != nil {
		return applicationRemoteRelation{}, errors.Errorf("generating offer connection UUID: %w", err)
	}
	relationID, err := sequencestate.NextValue(ctx, st, tx, domainrelation.SequenceNamespace)
	if err != nil {
		return applicationRemoteRelation{}, errors.Errorf("getting next relation id: %w", err)
	}

	rel := relation{
		UUID:       relationUUID.String(),
		LifeID:     int(life.Alive),
		RelationID: relationID,
	}
	charmScope := charmScope{
		Name: string(internalcharm.ScopeGlobal),
	}
	insertRelation := `
INSERT INTO relation (uuid, life_id, relation_id, scope_id)
SELECT $relation.uuid, $relation.life_id, $relation.relation_id, id
FROM   charm_relation_scope 
WHERE  name = $charmScope.name;`
	insertRelationStmt, err := st.Prepare(insertRelation, relation{}, charmScope)
	if err != nil {
		return applicationRemoteRelation{}, errors.Capture(err)
	}

	insertApplicationRemoteRelation := `
INSERT INTO application_remote_relation (relation_uuid, consumer_relation_uuid)
VALUES ($applicationRemoteRelation.*);`
	applicationRemoteRel := applicationRemoteRelation{
		RelationUUID:         relationUUID.String(),
		ConsumerRelationUUID: consumerRelationUUID,
	}
	insertRemoteRelationStmt, err := st.Prepare(insertApplicationRemoteRelation, applicationRemoteRel)
	if err != nil {
		return applicationRemoteRelation{}, errors.Capture(err)
	}

	// First insert the synthetic relation in the relation table.
	if err := tx.Query(ctx, insertRelationStmt, rel, charmScope).Run(); err != nil {
		return applicationRemoteRelation{}, errors.Errorf("inserting remote relation record: %w", err)
	}

	// Now insert the application remote relation record.
	if err := tx.Query(ctx, insertRemoteRelationStmt, applicationRemoteRel).Run(); err != nil {
		return applicationRemoteRelation{}, errors.Errorf("inserting application remote relation record: %w", err)
	}

	return applicationRemoteRel, nil
}

func (st *State) insertOfferConnection(
	ctx context.Context,
	tx *sqlair.TX,
	offerUUID string,
	applicationRemoteRelationUUID string,
) (string, error) {
	// Create an offer connection for this consumer.
	offerConnectionUUID, err := internaluuid.NewUUID()
	if err != nil {
		return "", errors.Errorf("generating offer connection UUID: %w", err)
	}

	insertOfferConnection := `
INSERT INTO offer_connection (uuid, offer_uuid, application_remote_relation_uuid, username)
VALUES ($offerConnection.*);`

	offerConn := offerConnection{
		UUID:                          offerConnectionUUID.String(),
		OfferUUID:                     offerUUID,
		ApplicationRemoteRelationUUID: applicationRemoteRelationUUID,
		Username:                      "consumer-user",
	}

	var emptyOfferConnection offerConnection
	insertOfferConnStmt, err := st.Prepare(insertOfferConnection, emptyOfferConnection)
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := tx.Query(ctx, insertOfferConnStmt, offerConn).Run(); err != nil {
		return "", errors.Errorf("inserting offer connection record: %w", err)
	}

	return offerConnectionUUID.String(), nil
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

func (st *State) nextRemoteApplicationConsumerVersion(
	ctx context.Context,
	tx *sqlair.TX,
	offerUUID string,
) (uint64, error) {

	namespace := domainsequence.MakePrefixNamespace(crossmodelrelation.ApplicationRemoteConsumerSequenceNamespace, offerUUID)
	nextVersion, err := sequencestate.NextValue(ctx, st, tx, namespace)
	if err != nil {
		return 0, errors.Errorf("getting next remote application consumer version: %w", err)
	}
	return nextVersion, nil
}

func (st *State) insertRemoteApplicationOffererStatus(
	ctx context.Context,
	tx *sqlair.TX,
	appID string,
	sts status.StatusInfo[status.WorkloadStatusType],
) error {
	insertQuery := `
INSERT INTO application_remote_offerer_status (*) VALUES ($remoteApplicationStatus.*);
`

	insertStmt, err := st.Prepare(insertQuery, remoteApplicationStatus{})
	if err != nil {
		return errors.Errorf("preparing insert query: %w", err)
	}

	statusID, err := status.EncodeWorkloadStatus(sts.Status)
	if err != nil {
		return errors.Errorf("encoding status: %w", err)
	}

	if err := tx.Query(ctx, insertStmt, remoteApplicationStatus{
		RemoteApplicationUUID: appID,
		StatusID:              statusID,
		Message:               sts.Message,
		Data:                  sts.Data,
		UpdatedAt:             sts.Since,
	}).Run(); err != nil {
		return errors.Errorf("inserting status: %w", err)
	}
	return nil
}

// insertApplicationEndpointBindings inserts database records for an
// application's endpoints (`application_endpoint`).
//
// It gets the relation defined in the charm and inserts all the endpoints
// into the default alpha space.
func (st *State) insertApplicationEndpointBindings(ctx context.Context, tx *sqlair.TX, appUUID, charmUUID string) error {
	relations, err := st.getCharmRelationNames(ctx, tx, charmUUID)
	if err != nil {
		return errors.Errorf("getting charm relation names: %w", err)
	}

	if err := st.insertApplicationRelationEndpointBindings(ctx, tx, appUUID, relations); err != nil {
		return errors.Errorf("inserting application endpoint: %w", err)
	}
	return nil
}

// insertApplicationRelationEndpointBindings inserts an application endpoint
// binding into the database, associating it with a relation and space.
func (st *State) insertApplicationRelationEndpointBindings(
	ctx context.Context,
	tx *sqlair.TX,
	appID string,
	relations []charmRelationName,
) error {
	if len(relations) == 0 {
		return nil
	}

	stmt, err := st.Prepare(
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

		inserts[i] = setApplicationEndpointBinding{
			UUID:          uuid.String(),
			ApplicationID: appID,
			RelationUUID:  relation.UUID,
		}
	}

	return tx.Query(ctx, stmt, inserts).Run()
}

// getCharmRelationNames retrieves a list of charm relation names from the
// database based on the provided parameters.
func (st *State) getCharmRelationNames(ctx context.Context, tx *sqlair.TX, charmUUID string) ([]charmRelationName, error) {
	uuid := uuid{UUID: charmUUID}
	stmt, err := st.Prepare(`
SELECT &charmRelationName.* 
FROM charm_relation
WHERE charm_relation.charm_uuid = $uuid.uuid
`, uuid, charmRelationName{})
	if err != nil {
		return nil, errors.Errorf("preparing fetch charm relation: %w", err)
	}
	var relations []charmRelationName
	if err := tx.Query(ctx, stmt, uuid).GetAll(&relations); err != nil && !errors.Is(err,
		sqlair.ErrNoRows) {
		return nil, errors.Errorf("fetching charm relation: %w", err)
	}
	return relations, nil
}

// getApplicationUUIDByOfferUUID retrieves the application UUID for the given
// offer UUID.
func (st *State) getApplicationUUIDByOfferUUID(
	ctx context.Context,
	tx *sqlair.TX,
	offerUUID string,
) (string, error) {
	ident := offerConnectionQuery{
		OfferUUID: offerUUID,
	}

	existsQueryStmt, err := st.Prepare(`
SELECT a.uuid AS &uuid.uuid
FROM   application AS a
JOIN   application_endpoint AS ae ON ae.application_uuid = a.uuid
JOIN   offer_endpoint AS oe ON oe.endpoint_uuid = ae.uuid
WHERE  oe.offer_uuid = $offerConnectionQuery.offer_uuid
`, uuid{}, ident)
	if err != nil {
		return "", errors.Capture(err)
	}

	var res uuid
	if err = tx.Query(ctx, existsQueryStmt, ident).Get(&res); errors.Is(err, sqlair.ErrNoRows) {
		return "", applicationerrors.ApplicationNotFound
	} else if err != nil {
		return "", errors.Errorf("retrieving application UUID from offer %q: %w", offerUUID, err)
	}

	return res.UUID, nil
}

// checkRemoteApplicationExists checks if a remote application with the given
// offer UUID, remote application UUID and remote relation UUID already exists.
func (st *State) checkRemoteApplicationExists(
	ctx context.Context,
	tx *sqlair.TX,
	offerUUID string,
	remoteApplicationUUID string,
	remoteRelationUUID string,
) error {
	consumerRelUUID := consumerRelationUUID{ConsumerRelationUUID: remoteRelationUUID}
	consumerRelationExistsStmt, err := st.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM  application_remote_relation
WHERE consumer_relation_uuid = $consumerRelationUUID.consumer_relation_uuid
`, countResult{}, consumerRelUUID)
	if err != nil {
		return errors.Capture(err)
	}

	offerConnectionOfferUUID := offerConnectionQuery{OfferUUID: offerUUID}
	consumerAppUUID := consumerApplicationUUID{ConsumerApplicationUUID: remoteApplicationUUID}
	consumerApplicationExistsStmt, err := st.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM  application_remote_consumer AS arc
JOIN  offer_connection AS oc ON oc.uuid = arc.offer_connection_uuid
WHERE arc.consumer_application_uuid = $consumerApplicationUUID.consumer_application_uuid
AND   oc.offer_uuid = $offerConnectionQuery.offer_uuid
`, countResult{}, consumerAppUUID, offerConnectionOfferUUID)
	if err != nil {
		return errors.Capture(err)
	}

	var result countResult
	// First check if the consumer relation already exists.
	if err := tx.Query(ctx, consumerRelationExistsStmt, consumerRelUUID).Get(&result); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking if consumer relation %q exists: %w", remoteRelationUUID, err)
	}
	if result.Count > 0 {
		return crossmodelrelationerrors.RemoteRelationAlreadyRegistered
	}

	// Now check if the consumer application, related with the offer UUID
	// already exists.
	if err := tx.Query(ctx, consumerApplicationExistsStmt, consumerAppUUID, offerConnectionOfferUUID).Get(&result); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking if consumer application %q exists: %w", remoteApplicationUUID, err)
	}
	if result.Count > 0 {
		return crossmodelrelationerrors.RemoteRelationAlreadyRegistered
	}

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

// checkApplicationRemoteOffererDoesNotExist checks if an offer with the given
// UUID already exists. It returns true if the offer exists, false if it does
// not.
func (st *State) checkApplicationRemoteOffererDoesNotExist(
	ctx context.Context,
	tx *sqlair.TX,
	offerUUID string,
) error {
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

// CheckOfferByUUID checks if an offer with the given UUID exists.
// Returns [crossmodelrelationerrors.OfferNotFound] if the offer is not found.
func (st *State) CheckOfferByUUID(ctx context.Context, offerUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	var result countResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {

		uuid := uuid{UUID: offerUUID}
		existsQueryStmt, err := st.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM offer
WHERE uuid = $uuid.uuid
`, uuid, result)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, existsQueryStmt, uuid).Get(&result); err != nil {
			return errors.Errorf("checking if offer %q exists: %w", offerUUID, err)
		}

		return nil
	})

	if err != nil {
		return errors.Capture(err)
	} else if result.Count == 0 {
		return crossmodelrelationerrors.OfferNotFound
	}

	return nil
}

func (st *State) addCharm(ctx context.Context, tx *sqlair.TX, uuid string, ch charm.Charm) error {
	if err := st.addCharmState(ctx, tx, uuid, ch); err != nil {
		return errors.Capture(err)
	}

	if err := st.addCharmMetadata(ctx, tx, uuid, ch.Metadata); err != nil {
		return errors.Capture(err)
	}

	if err := st.addCharmRelations(ctx, tx, uuid, ch.Metadata); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) addCharmState(
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
	charmStmt, err := st.Prepare(charmQuery, chState)
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, charmStmt, chState).Run(); err != nil {
		return errors.Errorf("inserting charm state: %w", err)
	}

	return nil
}

func (st *State) addCharmMetadata(
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
	stmt, err := st.Prepare(query, encodedMetadata)
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedMetadata).Run(); err != nil {
		return errors.Errorf("inserting charm metadata: %w", err)
	}

	return nil
}

func (st *State) addCharmRelations(ctx context.Context, tx *sqlair.TX, uuid string, metadata charm.Metadata) error {
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
	stmt, err := st.Prepare(query, setCharmRelation{})
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

func decodeMacaroon(data []byte) (*macaroon.Macaroon, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var m macaroon.Macaroon
	if err := m.UnmarshalJSON(data); err != nil {
		return nil, errors.Errorf("unmarshalling macaroon: %w", err)
	}
	return &m, nil
}
