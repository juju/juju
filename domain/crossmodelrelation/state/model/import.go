// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/crossmodelrelation/internal"
	"github.com/juju/juju/domain/life"
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
	// TODO: Implement relation key lookup in the state
	return "", errors.Errorf("relation key %q not found", key)
}

// ImportRemoteApplicationSecretGrants imports secrets granted by offerer applications
// to consumer applications in the offerer model.
func (st *State) ImportRemoteApplicationSecretGrants(ctx context.Context,
	values []internal.RemoteApplicationSecretGrant) error {
	return errors.Errorf("not implemented")
}

// ImportRemoteSecretConsumers imports secret consumers in the consumer model that
// consume secrets granted by offerer applications in the offerer model.
func (st *State) ImportRemoteSecretConsumers(ctx context.Context,
	values []internal.RemoteUnitConsumer) error {
	return errors.Errorf("not implemented")
}

// GetUnitUUIDByName returns the unit UUID for the named unit in the named application.
func (st *State) GetUnitUUIDByName(ctx context.Context, unitName string) (string, error) {
	return "", errors.Errorf("not implemented")
}

// ImportRemoteSecret imports a remote secret on the consumer model.
func (st *State) ImportRemoteSecret(ctx context.Context, secret internal.RemoteSecret) error {
	return errors.Errorf("not implemented")
}
