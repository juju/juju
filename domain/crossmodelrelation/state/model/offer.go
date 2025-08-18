// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/crossmodelrelation/internal"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// CreateOffer creates an offer and links the endpoints to it.
func (st *State) CreateOffer(
	ctx context.Context,
	args internal.CreateOfferArgs,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	createOfferStmt, err := st.Prepare(`
INSERT INTO offer (*) VALUES ($nameAndUUID.*)`, nameAndUUID{})
	if err != nil {
		return errors.Errorf("preparing insert offer query: %w", err)
	}
	offer := nameAndUUID{Name: args.OfferName, UUID: args.UUID.String()}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		applicationUUID, err := st.getApplicationUUID(ctx, tx, args.ApplicationName)
		if err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, createOfferStmt, offer).Run()
		if err != nil {
			return errors.Errorf("inserting offer row for %q: %w", args.OfferName, err)
		}

		if err = st.createOfferEndpoints(ctx, tx, args.UUID.String(), applicationUUID, args.Endpoints); err != nil {
			return errors.Errorf("offer %q: %w", args.OfferName, err)
		}

		return nil
	})

	return errors.Capture(err)
}

// DeleteFailedOffer deletes the provided offer, when adding permissions
// failed. Assumes that the offer is never used, no checking of relations
// is required.
func (st *State) DeleteFailedOffer(
	ctx context.Context,
	offerUUID internaluuid.UUID,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	deleteOfferStmt, err := st.Prepare(`
DELETE FROM offer
WHERE  uuid = $uuid.uuid`, uuid{})
	if err != nil {
		return errors.Errorf("preparing delete offer query: %w", err)
	}

	offer := uuid{UUID: offerUUID.String()}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {

		if err = st.deleteOfferEndpoints(ctx, tx, offerUUID.String()); err != nil {
			return nil
		}

		err = tx.Query(ctx, deleteOfferStmt, offer).Run()
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("deleting offer %q: %w", offerUUID, err)
		}
		return nil
	})

	return errors.Capture(err)
}

// UpdateOffer updates the endpoints of the given offer.
func (st *State) UpdateOffer(
	ctx context.Context,
	offerName string,
	offerEndpoints []string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		offerUUID, applicationUUID, err := st.getOfferAndApplicationUUID(ctx, tx, offerName)
		if err != nil {
			return err
		}

		// Delete all the current offer endpoints and create new ones.
		// TODO (cmr) verify that the endpoint is not in use as a
		// relation before making updates.

		if err = st.deleteOfferEndpoints(ctx, tx, offerUUID); err != nil {
			return errors.Errorf("offer %q: %w", offerName, err)
		}

		if err = st.createOfferEndpoints(ctx, tx, offerUUID, applicationUUID, offerEndpoints); err != nil {
			return errors.Errorf("offer %q: %w", offerName, err)
		}
		return nil
	})

	return errors.Capture(err)
}

func (st *State) getApplicationUUID(ctx context.Context, tx *sqlair.TX, appName string) (string, error) {
	appID := nameAndUUID{
		Name: appName,
	}

	// Prepare the SQL statement to retrieve the application UUID.
	stmt, err := st.Prepare(`
SELECT &nameAndUUID.uuid
FROM   application            
WHERE  name = $nameAndUUID.name
`, appID)
	if err != nil {
		return "", errors.Errorf("preparing application uuid query: %w", err)
	}

	// Execute the SQL transaction.
	err = tx.Query(ctx, stmt, appID).Get(&appID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", applicationerrors.ApplicationNotFound
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return appID.UUID, nil
}

func (st *State) getEndpointUUIDs(ctx context.Context, tx *sqlair.TX, appUUID string, endpoints []string) ([]string, error) {
	type dbStrings []string

	// Prepare the SQL statement to retrieve the endpoint UUIDs.
	stmt, err := st.Prepare(`
SELECT ae.application_endpoint_uuid AS &uuid.uuid
FROM   v_application_endpoint AS ae            
WHERE  ae.application_uuid = $uuid.uuid
AND    ae.endpoint_name IN ($dbStrings[:])
`, uuid{}, dbStrings{})
	if err != nil {
		return nil, errors.Errorf("preparing application endpoint query: %w", err)
	}

	result := []uuid{}

	// Execute the SQL transaction.
	err = tx.Query(ctx, stmt, uuid{UUID: appUUID}, dbStrings(endpoints)).GetAll(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("%q: %w", strings.Join(endpoints, ","), applicationerrors.EndpointNotFound)
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	if len(result) != len(endpoints) {
		return nil, errors.Errorf("not all endpoints found %q for application %q",
			strings.Join(endpoints, ", "),
			appUUID,
		).Add(crossmodelrelationerrors.MissingEndpoints)
	}

	return transform.Slice(result, func(in uuid) string {
		return in.UUID
	}), nil
}

func (st *State) getOfferAndApplicationUUID(ctx context.Context, tx *sqlair.TX, offerName string) (string, string, error) {
	// Prepare the SQL statement to retrieve the application UUID.
	stmt, err := st.Prepare(`
SELECT (ae.application_uuid, o.uuid) AS (&offerAndApplicationUUID.*)
FROM   offer AS o
JOIN   offer_endpoint AS oe ON o.uuid = oe.offer_uuid
JOIN   v_application_endpoint_uuid AS ae ON oe.endpoint_uuid = ae.uuid
WHERE  o.name = $name.name
`, name{}, offerAndApplicationUUID{})
	if err != nil {
		return "", "", errors.Errorf("preparing offer uuid query: %w", err)
	}

	offer := name{
		Name: offerName,
	}
	var result offerAndApplicationUUID
	// Execute the SQL transaction.
	err = tx.Query(ctx, stmt, offer).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", "", errors.Errorf("%q: %w", offerName, crossmodelrelationerrors.OfferNotFound)
	} else if err != nil {
		return "", "", errors.Capture(err)
	}

	return result.UUID, result.ApplicationUUID, nil
}

func (st *State) deleteOfferEndpoints(ctx context.Context, tx *sqlair.TX, offerUUID string) error {
	offer := uuid{
		UUID: offerUUID,
	}

	deleteOfferEndpointStmt, err := st.Prepare(`
DELETE FROM offer_endpoint
WHERE  offer_uuid = $uuid.uuid`, uuid{})
	if err != nil {
		return errors.Errorf("preparing delete offer_endpoint query: %w", err)
	}

	err = tx.Query(ctx, deleteOfferEndpointStmt, offer).Run()
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("deleting offer_endpoints of %q : %w", offerUUID, err)
	}

	return nil
}

func (st *State) createOfferEndpoints(ctx context.Context, tx *sqlair.TX, offerUUID, applicationUUID string, endpoints []string) error {
	endpointUUIDs, err := st.getEndpointUUIDs(ctx, tx, applicationUUID, endpoints)
	if err != nil {
		return errors.Capture(err)
	}

	createOfferEndpointStmt, err := st.Prepare(`
INSERT INTO offer_endpoint (*) VALUES ($offerEndpoint.*)`, offerEndpoint{})
	if err != nil {
		return errors.Errorf("preparing insert offer_endpoint query: %w", err)
	}

	offerEndpoints := transform.Slice(endpointUUIDs, func(in string) offerEndpoint {
		return offerEndpoint{OfferUUID: offerUUID, EndpointUUID: in}
	})

	err = tx.Query(ctx, createOfferEndpointStmt, offerEndpoints).Run()
	if err != nil {
		return errors.Errorf("inserting offer_endpoint rows: %w", err)
	}

	return nil
}
