// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/domain/crossmodelrelation"
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
// migrated to the current model. These are applications live in the consumer
// model that this model is consuming from other models. The offerer application
// is the synthetic application created in the consumer model to represent the
// remote application being offered.
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
	// Generate UUIDs for the application, charm, and remote app record.
	applicationUUID, err := internaluuid.NewUUID()
	if err != nil {
		return errors.Errorf("generating application UUID: %w", err)
	}
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
		ApplicationUUID: applicationUUID.String(),
		CharmUUID:       charmUUID.String(),
		Charm:           offerer.SyntheticCharm,
	}); err != nil {
		return errors.Errorf("inserting application: %w", err)
	}

	// Create synthetic units for this remote application.
	// These units are needed for relations to be offererorted successfully.
	for _, unitName := range offerer.Units {
		if err := st.insertUnit(ctx, tx, unitName, applicationUUID.String(), charmUUID.String()); err != nil {
			return errors.Errorf("inserting synthetic unit %q: %w",
				unitName, err)
		}
	}

	// Insert the remote application offerer record.
	remoteApp := remoteApplicationOfferer{
		UUID:             remoteAppUUID.String(),
		LifeID:           life.Alive,
		ApplicationUUID:  applicationUUID.String(),
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
// migrated to the current model. These are applications live in the offerer
// model that this model is offering to other models. The consumer application
// is the synthetic application created in the offerer model to represent the
// remote application being offered.
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

	// Get the application UUID for which the offer UUID was created.
	_, offerApplicationUUID, err := st.getApplicationNameAndUUIDByOfferUUID(ctx, tx, consumer.OfferUUID)
	if err != nil {
		return errors.Capture(err)
	}

	if err := st.checkApplicationNotDead(ctx, tx, offerApplicationUUID); err != nil {
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

	charmUUID, err := internaluuid.NewUUID()
	if err != nil {
		return errors.Errorf("generating charm UUID: %w", err)
	}

	// Insert the application, along with the associated charm.
	if err := st.insertApplication(ctx, tx, applicationName, insertApplicationArgs{
		ApplicationUUID: consumer.SyntheticApplicationUUID,
		CharmUUID:       charmUUID.String(),
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
		ApplicationOneUUID: consumer.SyntheticApplicationUUID,
		EndpointOneName:    consumer.ConsumerApplicationEndpoint,
		ApplicationTwoUUID: offerApplicationUUID,
		EndpointTwoName:    consumer.OffererApplicationEndpoint,
	}
	if err := st.insertRelationEndpoints(ctx, tx, relEndpointArgs); err != nil {
		return errors.Capture(err)
	}

	// Create an offer connection for this consumer.
	offerConnectionUUID, err := st.insertOfferConnection(ctx, tx,
		consumer.SyntheticApplicationUUID,
		consumer.OfferUUID,
		consumer.RelationUUID,
		consumer.Username,
	)
	if err != nil {
		return errors.Capture(err)
	}

	// Insert the remote application consumer record, this allows us to find
	// the synthetic application later.
	if err := st.insertRemoteApplicationConsumer(ctx, tx,
		offerConnectionUUID,
		offerApplicationUUID,
		consumer.ConsumerApplicationUUID,
		consumer.ConsumerModelUUID,
	); err != nil {
		return errors.Capture(err)
	}

	return nil
}
