// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	corerelation "github.com/juju/juju/core/relation"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/crossmodelrelation/internal"
	"github.com/juju/juju/domain/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// ImportOffers adds offers being migrated to the current model.
func (st *State) ImportOffers(ctx context.Context, imports []crossmodelrelation.OfferImport) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	applicationNames := transform.Slice(imports, func(in crossmodelrelation.OfferImport) string {
		return in.ApplicationName
	})
	uniqueApplicationNames := set.NewStrings(applicationNames...)

	createOffersStmt, err := st.Prepare(`
INSERT INTO offer (*) VALUES ($nameAndUUID.*)`, nameAndUUID{})
	if err != nil {
		return errors.Errorf("preparing insert offer query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appNamesUUIDs, err := st.getApplicationUUIDs(ctx, tx, uniqueApplicationNames.Values())
		if err != nil {
			return err
		}
		if len(appNamesUUIDs) != uniqueApplicationNames.Size() {
			return errors.Errorf("expected %d application uuids, got %d", len(appNamesUUIDs), len(applicationNames))
		}

		offersToAdd := transform.Slice(imports, func(in crossmodelrelation.OfferImport) nameAndUUID {
			return nameAndUUID{
				Name: in.Name,
				UUID: in.UUID.String(),
			}
		})

		err = tx.Query(ctx, createOffersStmt, offersToAdd).Run()
		if err != nil {
			return errors.Errorf("inserting import offer rows: %w", err)
		}

		for _, o := range imports {
			appUUID := appNamesUUIDs[o.ApplicationName]
			err := st.createOfferEndpoints(ctx, tx, o.UUID.String(), appUUID, o.Endpoints)
			if err != nil {
				return errors.Errorf("inserting import offer endpoints for %q: %w", o.Name, err)
			}
		}

		return nil
	})
	return errors.Capture(err)
}

// ImportRemoteApplicationOfferers adds remote application offerers being
// migrated to the current model. These are applications that live in the
// consumer model standing in for applications from other models. The offerer
// application is the synthetic application created in the consumer model to
// represent the remote application being offered.
func (st *State) ImportRemoteApplicationOfferers(ctx context.Context, imports []crossmodelrelation.RemoteApplicationOffererImport) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, offerer := range imports {
			if err := st.importRemoteApplicationOfferer(ctx, tx, offerer); err != nil {
				return errors.Errorf("importing remote application offerer %q: %w", offerer.Name, err)
			}
		}

		return nil
	})
	return errors.Capture(err)
}

func (st *State) importRemoteApplicationOfferer(ctx context.Context, tx *sqlair.TX, offerer crossmodelrelation.RemoteApplicationOffererImport) error {
	charmUUID, err := internaluuid.NewUUID()
	if err != nil {
		return errors.Errorf("generating charm UUID: %w", err)
	}
	remoteAppUUID, err := internaluuid.NewUUID()
	if err != nil {
		return errors.Errorf("generating remote application UUID: %w", err)
	}

	// Insert the application (which also inserts the charm).
	// The synthetic charm is pre-built in the service layer.
	if err := st.insertApplication(ctx, tx, offerer.Name, insertApplicationArgs{
		ApplicationUUID: offerer.OffererApplicationUUID,
		CharmUUID:       charmUUID.String(),
		Charm:           offerer.SyntheticCharm,
	}); err != nil {
		return errors.Errorf("inserting application: %w", err)
	}

	// Create synthetic units for this remote application.
	// These units are needed for relations to be imported successfully.
	for _, unitName := range offerer.Units {
		if err := st.insertUnit(ctx, tx, unitName, offerer.OffererApplicationUUID, charmUUID.String()); err != nil {
			return errors.Errorf("inserting synthetic unit %q: %w",
				unitName, err)
		}
	}

	// Insert the remote application offerer record.
	remoteApp := remoteApplicationOfferer{
		UUID:             remoteAppUUID.String(),
		LifeID:           life.Alive,
		ApplicationUUID:  offerer.OffererApplicationUUID,
		OfferUUID:        offerer.OfferUUID,
		OfferURL:         offerer.URL,
		OffererModelUUID: offerer.SourceModelUUID,
		Macaroon:         []byte(offerer.Macaroon),
	}

	insertRemoteAppStmt, err := st.Prepare(`
INSERT INTO application_remote_offerer (*)
VALUES ($remoteApplicationOfferer.*);
`, remoteApp)
	if err != nil {
		return errors.Errorf("preparing remote query: %w", err)
	}

	if err := tx.Query(ctx, insertRemoteAppStmt, remoteApp).Run(); err != nil {
		return errors.Errorf("inserting remote application offerer: %w", err)
	}

	return nil
}

// ImportRemoteApplicationConsumers adds remote application consumers being
// migrated to the current model. These are applications that live in the
// offerer model standing in for application from other models actively
// consuming the offer. The consumer application is the synthetic application
// created in the offerer model to represent the remote consuming application.
func (st *State) ImportRemoteApplicationConsumers(ctx context.Context, imports []crossmodelrelation.RemoteApplicationConsumerImport) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, consumer := range imports {
			if err := st.importRemoteApplicationConsumer(ctx, tx, consumer); err != nil {
				return errors.Errorf("importing remote application consumer %q: %w", consumer.Name, err)
			}
		}

		return nil
	})
	return errors.Capture(err)
}

func (st *State) importRemoteApplicationConsumer(ctx context.Context, tx *sqlair.TX, consumer crossmodelrelation.RemoteApplicationConsumerImport) error {
	applicationName := consumer.Name

	if err := st.checkApplicationNotDead(ctx, tx, consumer.OffererApplicationUUID); err != nil {
		return errors.Capture(err)
	}

	// If the relation already exists, return an error. All relations are
	// immutable, so we can only consume it once.
	if err := st.checkConsumerRelationExists(ctx, tx, consumer.RelationUUID); err != nil {
		return errors.Capture(err)
	}

	// Check if the application already exists.
	if err := st.checkApplicationNameAvailable(ctx, tx, applicationName); err != nil {
		return errors.Errorf("checking if application %q exists: %w", applicationName, err)
	}

	// Insert the application, along with the associated charm.
	if err := st.insertApplication(ctx, tx, applicationName, insertApplicationArgs{
		ApplicationUUID: consumer.ConsumerApplicationUUID,
		CharmUUID:       consumer.SyntheticCharmUUID,
		Charm:           consumer.SyntheticCharm,
	}); err != nil {
		return errors.Capture(err)
	}

	// Create the synthetic relation for this consumer.
	if err := st.insertSyntheticRelation(ctx, tx, consumer.RelationUUID); err != nil {
		return errors.Capture(err)
	}

	// Insert the joined status for the relation.
	if err := st.insertNewRelationStatus(ctx, tx, consumer.RelationUUID); err != nil {
		return errors.Capture(err)
	}

	// Create relation_Endpoints for the relation, maps relations to
	// application_endpoints.
	relEndpointArgs := addRelationEndpointArgs{
		RelationUUID:       consumer.RelationUUID,
		ApplicationOneUUID: consumer.ConsumerApplicationUUID,
		EndpointOneName:    consumer.ConsumerApplicationEndpoint,
		ApplicationTwoUUID: consumer.OffererApplicationUUID,
		EndpointTwoName:    consumer.OffererApplicationEndpoint,
	}
	if err := st.insertRelationEndpoints(ctx, tx, relEndpointArgs); err != nil {
		return errors.Capture(err)
	}

	// Create an offer connection for this consumer.
	offerConnectionUUID, err := st.insertOfferConnection(ctx, tx,
		consumer.ConsumerApplicationUUID,
		consumer.OfferUUID,
		consumer.RelationUUID,
		consumer.UserName,
	)
	if err != nil {
		return errors.Capture(err)
	}

	// Insert the remote application consumer record, this allows us to find
	// the synthetic application later.
	if err := st.insertRemoteApplicationConsumer(ctx, tx,
		offerConnectionUUID,
		consumer.OffererApplicationUUID,
		consumer.ConsumerApplicationUUID,
		consumer.ConsumerModelUUID,
	); err != nil {
		return errors.Capture(err)
	}

	// Create synthetic units for this remote application.
	for _, unitName := range consumer.Units {
		if err := st.insertUnit(ctx, tx, unitName, consumer.ConsumerApplicationUUID, consumer.SyntheticCharmUUID); err != nil {
			return errors.Errorf("inserting synthetic unit %q: %w",
				unitName, err)
		}
	}

	return nil
}

// GetApplicationUUIDByName returns the application UUID for the named
// application.
// The following errors may be returned:
// - [applicationerrors.ApplicationNotFound] if the application does not exist
func (st *State) GetApplicationUUIDByName(ctx context.Context, name string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var id string
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		id, err = st.getApplicationUUID(ctx, tx, name)
		return err
	}); err != nil {
		return "", errors.Capture(err)
	}
	return id, nil
}

// GetRelationUUIDByRelationKey retrieves the UUID of a relation using its relation key.
func (st *State) GetRelationUUIDByRelationKey(ctx context.Context, key corerelation.Key) (string, error) {
	eps := key.EndpointIdentifiers()
	if len(eps) != 2 {
		return "", errors.Errorf("relation key %q has %d endpoints, expected 2", key, len(eps))
	}

	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuid []uuid
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err = st.getRegularRelationUUIDByEndpointIdentifiers(
			ctx,
			tx,
			eps[0],
			eps[1],
		)
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	if len(uuid) > 1 {
		return "", errors.Errorf("found multiple relations for endpoint pair")
	}

	return uuid[0].UUID, nil
}

func (st *State) getRegularRelationUUIDByEndpointIdentifiers(
	ctx context.Context,
	tx *sqlair.TX,
	endpoint1, endpoint2 corerelation.EndpointIdentifier,
) ([]uuid, error) {
	var uuids []uuid
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
SELECT &uuid.*
FROM   relation r
JOIN   v_relation_endpoint_identifier e1 ON r.uuid = e1.relation_uuid
JOIN   v_relation_endpoint_identifier e2 ON r.uuid = e2.relation_uuid
WHERE  e1.application_name = $endpointIdentifier1.application_name 
AND    e1.endpoint_name    = $endpointIdentifier1.endpoint_name
AND    e2.application_name = $endpointIdentifier2.application_name 
AND    e2.endpoint_name    = $endpointIdentifier2.endpoint_name
`, uuid{}, e1, e2)
	if err != nil {
		return uuids, errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, e1, e2).GetAll(&uuids)
	if errors.Is(err, sqlair.ErrNoRows) {
		return uuids, relationerrors.RelationNotFound
	}
	return uuids, errors.Capture(err)
}

// ImportRemoteApplicationSecretGrants imports secrets granted by offerer applications
// to consumer applications in the offerer model.
func (st *State) ImportRemoteApplicationSecretGrants(ctx context.Context,
	values []internal.RemoteApplicationSecretGrant) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	insertStmt, err := st.Prepare(`
INSERT INTO secret_permission (*)
VALUES ($remoteSecretGrant.*)`, remoteSecretGrant{})
	if err != nil {
		return errors.Errorf("preparing insert secret grant query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, grant := range values {
			if err := tx.Query(ctx, insertStmt, remoteSecretGrant{
				SecretID:      grant.SecretID,
				RoleID:        domainsecret.RoleView,
				SubjectUUID:   grant.ApplicationUUID,
				SubjectTypeID: domainsecret.SubjectApplication,
				ScopeUUID:     grant.RelationUUID,
				ScopeTypeID:   domainsecret.ScopeRelation,
			}).Run(); err != nil {
				return errors.Errorf("inserting remote secret grant for application %q, relation %q: %w",
					grant.ApplicationName, grant.RelationKey, err)
			}
		}
		return nil
	})

	return errors.Capture(err)
}

// ImportRemoteSecretConsumers imports secret consumers in the consumer model that
// consume secrets granted by offerer applications in the offerer model.
func (st *State) ImportRemoteSecretConsumers(ctx context.Context,
	values []internal.RemoteUnitConsumer) error {
	for _, consumer := range values {
		if err := st.SaveSecretRemoteConsumer(
			ctx,
			&coresecrets.URI{ID: consumer.SecretID},
			consumer.Unit,
			coresecrets.SecretConsumerMetadata{
				CurrentRevision: consumer.CurrentRevision,
			},
		); err != nil {
			return errors.Errorf("saving remote secret consumer unit %q: %w", consumer.Unit, err)
		}
	}
	return nil
}

// ImportRemoteSecret imports a remote secret on the consumer model.
func (st *State) ImportRemoteSecret(ctx context.Context, secret internal.RemoteSecret) error {
	// TODO(gfouillet): reimplement remote secret import.
	//   By default there is no need to implement it, since those information will be populated again whenever the
	//   secret will be fetched again by the unit through a secret-get.
	//   Non imported date belongs to both table secret_unit_consumer and secret_reference, but can't be completely
	//   populated during the migration process: we don't have any solution to figure out the value of
	//   secret_reference.owner_application_uuid from the model description.
	//   I need to figure out if those data really need to be populated during migration process, and if it is
	//   acceptable to have an empty value for secret_reference.owner_application_uuid, at least until the next
	//   retrieval of the data by a unit.
	//   This will be done in a follow up PR.

	//	db, err := st.DB(ctx)
	//	if err != nil {
	//		return errors.Capture(err)
	//	}
	//
	//	// Remote secrets doesn't have a reference yet.
	//	secretRef := secretRef{ID: secret.SecretID}
	//	insertRemoteSecretStmt, err := st.Prepare(`
	//INSERT INTO secret (id)
	//VALUES ($secretRef.secret_id)`, secretRef)
	//	if err != nil {
	//		return errors.Capture(err)
	//	}
	//
	//	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
	//		err = tx.Query(ctx, insertRemoteSecretStmt, secretRef).Run()
	//		if err != nil {
	//			return errors.Errorf("inserting remote secret reference: %w", err)
	//		}
	//
	//		err = st.saveSecretConsumer(ctx, tx, &coresecrets.URI{
	//			SourceUUID: secret.SourceModelUUID,
	//			ID:         secret.SecretID,
	//		}, secret.UnitUUID, coresecrets.SecretConsumerMetadata{
	//			Label:           secret.Label,
	//			CurrentRevision: secret.CurrentRevision,
	//		})
	//		if err != nil {
	//			return errors.Errorf("saving remote secret consumer info: %w", err)
	//		}
	//
	//		return nil
	//	})
	//	if err != nil {
	//		return errors.Capture(err)
	//	}

	return nil
}
