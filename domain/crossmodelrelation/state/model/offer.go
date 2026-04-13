// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/offer"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/internal/errors"
)

// CreateOffer creates an offer and links the endpoints to it.
func (st *State) CreateOffer(
	ctx context.Context,
	args crossmodelrelation.CreateOfferArgs,
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

		if err := st.checkApplicationNotDead(ctx, tx, applicationUUID); err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, createOfferStmt, offer).Run(); err != nil {
			return errors.Errorf("inserting offer row for %q: %w", args.OfferName, err)
		}

		if err := st.createOfferEndpoints(ctx, tx, args.UUID.String(), applicationUUID, args.Endpoints); err != nil {
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
	offerUUID offer.UUID,
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

// GetOfferUUIDByRelationUUID returns the offer UUID corresponding to
// the cross model relation UUID, returning an error satisfying
// [crossmodelrelationerrors.OfferNotFound] if the relation is not found.
func (st *State) GetOfferUUIDByRelationUUID(ctx context.Context, relationUUID string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	remoteRelationUUID := remoteRelationUUID{
		UUID: relationUUID,
	}

	stmt, err := st.Prepare(`
SELECT &offerConnection.*
FROM   offer_connection oc
WHERE  oc.remote_relation_uuid = $remoteRelationUUID.uuid
`, remoteRelationUUID, offerConnection{})
	if err != nil {
		return "", errors.Capture(err)
	}

	var result offerConnection
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, remoteRelationUUID).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("offer for relation %q: %w", relationUUID, crossmodelrelationerrors.OfferNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		return err
	})
	return result.OfferUUID, err
}

// GetOfferUUID returns the offer uuid for provided name.
// Returns crossmodelrelationerrors.OfferNotFound of the offer is not found.
func (st *State) GetOfferUUID(ctx context.Context, name string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var offerUUID string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		offerUUID, _, err = st.getOfferAndApplicationUUID(ctx, tx, name)
		return err
	})

	return offerUUID, err
}

// GetConsumeDetails returns the offer uuid and endpoints necessary to
// consume the offer.
// Returns crossmodelrelationerrors.OfferNotFound of the offer is not found.
func (st *State) GetConsumeDetails(
	ctx context.Context,
	offerName string,
) (crossmodelrelation.ConsumeDetails, error) {
	var empty crossmodelrelation.ConsumeDetails
	db, err := st.DB(ctx)
	if err != nil {
		return empty, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT (o.uuid, cr.name, cr.interface, cr.capacity) AS (&consumeDetail.*),
       crr.name AS &consumeDetail.role
FROM   offer AS o
JOIN   offer_endpoint AS oe ON o.uuid = oe.offer_uuid
JOIN   application_endpoint AS ae ON oe.endpoint_uuid = ae.uuid
JOIN   application AS a ON ae.application_uuid = a.uuid
JOIN   charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
JOIN   charm_relation_role AS crr ON cr.role_id = crr.id
WHERE  o.name = $name.name
`, consumeDetail{}, name{})
	if err != nil {
		return empty, errors.Errorf("preparing consume detail query: %w", err)
	}

	var details []consumeDetail
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, name{Name: offerName}).GetAll(&details)
		if errors.Is(err, sqlair.ErrNoRows) {
			return crossmodelrelationerrors.OfferNotFound
		}
		return err
	})
	if err != nil {
		return empty, errors.Errorf("fetching consume details for %q: %w", offerName, err)
	}
	endpoints := transform.Slice(details, func(in consumeDetail) crossmodelrelation.OfferEndpoint {
		return crossmodelrelation.OfferEndpoint{
			Name:      in.EndpointName,
			Role:      in.EndpointRole,
			Interface: in.EndpointInterface,
			Limit:     in.EndpointLimit,
		}
	})
	return crossmodelrelation.ConsumeDetails{
		OfferUUID: details[0].OfferUUID,
		Endpoints: endpoints,
	}, nil
}

// GetOfferDetails returns the OfferDetail of every offer in the model.
// No error is returned if offers are found.
func (st *State) GetOfferDetails(
	ctx context.Context,
	filter crossmodelrelation.OfferFilter,
) ([]*crossmodelrelation.OfferDetail, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var offers offerDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if filter.Empty() {
			offers, err = st.getOfferDetails(ctx, tx)
			return err
		}
		offers, err = st.getFilteredOfferDetails(ctx, tx, filter)
		if err != nil {
			return errors.Capture(err)
		}

		if len(filter.OfferUUIDs) == 0 {
			return nil
		}

		result, err := st.getOfferDetailsForUUIDs(ctx, tx, filter.OfferUUIDs)
		if err != nil {
			return errors.Capture(err)
		}
		offers = append(offers, result...)

		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return offers.TransformToOfferDetails(), nil
}

func (st *State) getOfferDetails(ctx context.Context, tx *sqlair.TX) (offerDetails, error) {
	stmt, err := st.Prepare(`
SELECT &offerDetail.*
FROM   v_offer_detail
`, offerDetail{})
	if err != nil {
		return nil, errors.Errorf("preparing offer detail query: %w", err)
	}

	var offers offerDetails
	err = tx.Query(ctx, stmt).GetAll(&offers)
	if errors.Is(err, sqlair.ErrNoRows) {
		return offers, nil
	}
	if err != nil {
		return nil, errors.Errorf("fetching offer all details: %w", err)
	}
	return offers, nil
}

func (st *State) getOfferDetailsForUUIDs(ctx context.Context, tx *sqlair.TX, offerUUIDs []string) (offerDetails, error) {
	stmt, err := st.Prepare(`
SELECT &offerDetail.*
FROM   v_offer_detail
WHERE  offer_uuid IN ($uuids[:])
`, offerDetail{}, uuids{})
	if err != nil {
		return nil, errors.Errorf("preparing offer detail for UUID query: %w", err)
	}

	var offers offerDetails
	err = tx.Query(ctx, stmt, uuids(offerUUIDs)).GetAll(&offers)
	if errors.Is(err, sqlair.ErrNoRows) {
		return offers, nil
	}
	if err != nil {
		return nil, errors.Errorf("fetching offer details by uuid: %w", err)
	}
	return offers, nil
}

func (st *State) getFilteredOfferDetails(ctx context.Context, tx *sqlair.TX, input crossmodelrelation.OfferFilter) (offerDetails, error) {
	stmt, err := st.Prepare(`
SELECT &offerDetail.*
FROM   v_offer_detail
WHERE  offer_name = $offerFilter.offer_name
OR     application_name LIKE $offerFilter.application_name
OR     application_description LIKE $offerFilter.application_description
OR     endpoint_name = $offerFilter.endpoint_name
OR     endpoint_role = $offerFilter.endpoint_role
OR     endpoint_interface = $offerFilter.endpoint_interface
`, offerDetail{}, offerFilter{})
	if err != nil {
		return nil, errors.Errorf("preparing filtered offer detail query: %w", err)
	}

	filter, err := encodeOfferFilter(input)
	if err != nil {
		return nil, errors.Errorf("encoding offer filter: %w", err)
	}

	var result offerDetails
	for _, f := range filter {
		var offers []offerDetail
		err = tx.Query(ctx, stmt, f).GetAll(&offers)
		if errors.Is(err, sqlair.ErrNoRows) {
			// There is no guarantee of success with any filter.
			continue
		}
		if err != nil {
			return nil, errors.Errorf("fetching offer details by filter: %w", err)
		}
		result = append(result, offers...)
	}
	return result, nil
}

// encodeOfferFilter makes offerFilters, used to query the database,
// from [crossmodelrelation.OfferFilter]. The filter parameters are ORed
// together to find offers. Thus, the input can be split into multiple
// output.
//
// Application name and description filter values should be contained with
// the actual result. Setup their values to use the LIKE operator by adding
// an `%` before and after the word if provided.
func encodeOfferFilter(in crossmodelrelation.OfferFilter) ([]offerFilter, error) {
	result := make([]offerFilter, 0)
	if !in.EmptyModuloEndpoints() {
		var (
			applicationName, applicationDescription string
		)
		if in.ApplicationName != "" {
			applicationName = fmt.Sprintf("%%%s%%", in.ApplicationName)
		}
		if in.ApplicationDescription != "" {
			applicationDescription = fmt.Sprintf("%%%s%%", in.ApplicationDescription)
		}
		result = append(result, offerFilter{
			OfferName:              in.OfferName,
			ApplicationName:        applicationName,
			ApplicationDescription: applicationDescription,
		})
	}
	for _, endpoint := range in.Endpoints {
		result = append(result, offerFilter{
			EndpointName: endpoint.Name,
			Interface:    endpoint.Interface,
			Role:         string(endpoint.Role),
		})
	}
	return result, nil
}

func (st *State) getApplicationUUID(ctx context.Context, tx *sqlair.TX, appName string) (string, error) {
	stmt, err := st.Prepare(`
SELECT uuid AS &uuid.uuid
FROM   application
WHERE  name = $name.name
`, name{}, uuid{})
	if err != nil {
		return "", errors.Errorf("preparing application uuid query: %w", err)
	}

	var result uuid
	if err := tx.Query(ctx, stmt, name{Name: appName}).Get(&result); errors.Is(err, sqlair.ErrNoRows) {
		return "", applicationerrors.ApplicationNotFound
	} else if err != nil {
		return "", errors.Capture(err)
	}
	return result.UUID, nil
}

func (st *State) getApplicationUUIDs(ctx context.Context, tx *sqlair.TX, appNames []string) (map[string]string, error) {
	type names []string

	// Prepare the SQL statement to retrieve the application UUID.
	stmt, err := st.Prepare(`
SELECT &nameAndUUID.*
FROM   application            
WHERE  name IN ($names[:])
`, nameAndUUID{}, names{})
	if err != nil {
		return nil, errors.Errorf("preparing application uuid query: %w", err)
	}

	// Execute the SQL transaction.
	var appIDs []nameAndUUID
	err = tx.Query(ctx, stmt, names(appNames)).GetAll(&appIDs)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, applicationerrors.ApplicationNotFound
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.SliceToMap(appIDs, func(in nameAndUUID) (string, string) {
		return in.Name, in.UUID
	}), nil
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

// GetOfferConnections returns the connection details for all offers with the
// given UUIDs. An empty result is returned if no connections are found.
func (st *State) GetOfferConnections(
	ctx context.Context,
	offerUUIDs []string,
) ([]crossmodelrelation.OfferConnectionDetail, error) {
	if len(offerUUIDs) == 0 {
		return nil, nil
	}
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Query connection details: relation id, username, consumer model UUID,
	// endpoint name, and relation status.
	connStmt, err := st.Prepare(`
SELECT oc.offer_uuid            AS &offerConnectionDetail.offer_uuid,
       r.relation_id            AS &offerConnectionDetail.relation_id,
       oc.username              AS &offerConnectionDetail.username,
       arc.consumer_model_uuid  AS &offerConnectionDetail.consumer_model_uuid,
       cr.name                  AS &offerConnectionDetail.endpoint_name,
       rst.name                 AS &offerConnectionDetail.status,
       rs.message               AS &offerConnectionDetail.message,
       rs.updated_at            AS &offerConnectionDetail.updated_at
FROM   offer_connection AS oc
JOIN   relation AS r ON oc.remote_relation_uuid = r.uuid
JOIN   application_remote_consumer AS arc ON oc.uuid = arc.offer_connection_uuid
JOIN   relation_endpoint AS re ON r.uuid = re.relation_uuid
JOIN   application_endpoint AS ae
       ON re.endpoint_uuid = ae.uuid
       AND ae.application_uuid = arc.offerer_application_uuid
JOIN   charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
JOIN   relation_status AS rs ON r.uuid = rs.relation_uuid
JOIN   relation_status_type AS rst ON rs.relation_status_type_id = rst.id
WHERE  oc.offer_uuid IN ($uuids[:])
`, offerConnectionDetail{}, uuids{})
	if err != nil {
		return nil, errors.Errorf("preparing offer connection detail query: %w", err)
	}

	// Query ingress subnets separately to avoid row multiplication.
	ingressStmt, err := st.Prepare(`
SELECT oc.offer_uuid   AS &offerConnectionIngress.offer_uuid,
       r.relation_id   AS &offerConnectionIngress.relation_id,
       rni.cidr         AS &offerConnectionIngress.cidr
FROM   offer_connection AS oc
JOIN   relation AS r ON oc.remote_relation_uuid = r.uuid
JOIN   relation_network_ingress AS rni ON oc.remote_relation_uuid = rni.relation_uuid
WHERE  oc.offer_uuid IN ($uuids[:])
`, offerConnectionIngress{}, uuids{})
	if err != nil {
		return nil, errors.Errorf("preparing offer connection ingress query: %w", err)
	}

	var connDetails []offerConnectionDetail
	var ingressRows []offerConnectionIngress

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Fetch connection details.
		err = tx.Query(ctx, connStmt, uuids(offerUUIDs)).GetAll(&connDetails)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Errorf("fetching offer connection details: %w", err)
		}

		// Fetch ingress subnets.
		err = tx.Query(ctx, ingressStmt, uuids(offerUUIDs)).GetAll(&ingressRows)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Errorf("fetching offer connection ingress: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Build a map of (offerUUID, relationID) → ingress CIDRs.
	type connKey struct {
		OfferUUID  string
		RelationID int
	}
	ingressMap := make(map[connKey][]string)
	for _, row := range ingressRows {
		key := connKey{OfferUUID: row.OfferUUID, RelationID: row.RelationID}
		ingressMap[key] = append(ingressMap[key], row.CIDR)
	}

	// Convert to domain types.
	return transform.Slice(connDetails, func(detail offerConnectionDetail) crossmodelrelation.OfferConnectionDetail {
		key := connKey{OfferUUID: detail.OfferUUID, RelationID: detail.RelationID}
		res := crossmodelrelation.OfferConnectionDetail{
			OfferUUID:       detail.OfferUUID,
			SourceModelUUID: detail.ConsumerModelUUID,
			RelationID:      detail.RelationID,
			Username:        detail.Username,
			Endpoint:        detail.EndpointName,
			Status:          detail.Status,
			StatusSince:     detail.StatusSince,
			IngressSubnets:  ingressMap[key],
		}
		if detail.Message.Valid {
			res.Message = detail.Message.String
		}
		return res
	}), nil
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
