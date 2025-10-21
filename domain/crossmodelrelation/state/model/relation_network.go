// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/errors"
)

// AddRelationNetworkEgress adds egress network CIDRs for the specified
// relation. The CIDRs are added to the relation_network_egress table.
// If a CIDR already exists for the relation, it will be silently ignored
// due to the primary key constraint.
//
// It returns a [relationerrors.RelationNotFound] if the provided relation does
// not exist.
func (st *State) AddRelationNetworkEgress(ctx context.Context, relationKey corerelation.Key, cidrs ...string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	endpoints := relationKey.EndpointIdentifiers()

	// Defensive check: this should be validated at the service layer, but we
	// verify here as well to ensure data integrity.
	if len(endpoints) != 2 {
		return errors.Errorf("relation must have exactly 2 endpoints, got %d", len(endpoints))
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get the relation UUID using the key
		relationUUID, err := st.getRegularRelationUUIDByEndpointIdentifiers(ctx, tx, endpoints[0], endpoints[1])
		if err != nil {
			return errors.Capture(err)
		}

		// Insert each CIDR
		insertStmt, err := st.Prepare(`
INSERT INTO relation_network_egress (relation_uuid, cidr)
VALUES ($relationNetworkEgress.*)
ON CONFLICT (relation_uuid, cidr) DO NOTHING
`, relationNetworkEgress{})
		if err != nil {
			return errors.Errorf("preparing insert statement: %w", err)
		}

		for _, cidr := range cidrs {
			entry := relationNetworkEgress{
				RelationUUID: relationUUID,
				CIDR:         cidr,
			}
			if err := tx.Query(ctx, insertStmt, entry).Run(); err != nil {
				return errors.Errorf("inserting CIDR %q for relation %q: %w", cidr, relationKey.String(), err)
			}
		}
		return nil
	})
}

// AddRelationNetworkIngress adds ingress network CIDRs for the specified
// relation.
// It returns a [relationerrors.RelationNotFound] if the provided relation does
// not exist.
func (st *State) AddRelationNetworkIngress(ctx context.Context, relationUUID string, cidrs []string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	insertStmt, err := st.Prepare(`
INSERT INTO relation_network_ingress (*)
VALUES ($relationNetworkIngress.*)
`, relationNetworkIngress{})
	if err != nil {
		return errors.Errorf("preparing insert relation network ingress query: %w", err)
	}

	var ingress []relationNetworkIngress
	for _, cidr := range cidrs {
		ingress = append(ingress, relationNetworkIngress{
			RelationUUID: relationUUID,
			CIDR:         cidr,
		})
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		relationLife, err := st.getRelationLife(ctx, tx, relationUUID)
		if err != nil {
			return errors.Capture(err)
		} else if life.IsNotAlive(relationLife) {
			return relationerrors.RelationNotAlive
		}

		if err := tx.Query(ctx, insertStmt, ingress).Run(); err != nil {
			return errors.Errorf("inserting relation network ingress for relation %q with CIDR %v: %w", relationUUID, cidrs, err)
		}
		return nil
	})

	return errors.Capture(err)
}

// GetRelationNetworkIngress retrieves all ingress network CIDRs for the
// specified relation.
// It returns a [relationerrors.RelationNotFound] if the provided relation does
// not exist.
// It returns a [relationerrors.RelationNotAlive] if the provided relation is
// dead.
func (st *State) GetRelationNetworkIngress(ctx context.Context, relationUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	selectStmt, err := st.Prepare(`
SELECT &cidr.*
FROM   relation_network_ingress
WHERE  relation_uuid = $uuid.uuid
`, cidr{}, uuid{})
	if err != nil {
		return nil, errors.Errorf("preparing select relation network ingress query: %w", err)
	}

	var cidrs []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// We ignore the returned life value here, we just want to check
		// that the relation exists and return its ingress networks even if
		// not alive.
		if _, err := st.getRelationLife(ctx, tx, relationUUID); err != nil {
			return errors.Capture(err)
		}

		var results []cidr
		if err := tx.Query(ctx, selectStmt, uuid{UUID: relationUUID}).GetAll(&results); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("retrieving relation network ingress for relation %q: %w", relationUUID, err)
		}

		cidrs = make([]string, len(results))
		for i, result := range results {
			cidrs[i] = result.CIDR
		}
		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	return cidrs, nil
}

// NamespaceForRelationIngressNetworksWatcher returns the namespace of the
// relation_network_ingress table, used for the watcher.
func (st *State) NamespaceForRelationIngressNetworksWatcher() string {
	return "relation_network_ingress"
}

// getRelationLife retrieves the life value for the specified relation.
// It returns a [relationerrors.RelationNotFound] if the relation does not
func (st *State) getRelationLife(ctx context.Context, tx *sqlair.TX, relationUUID string) (life.Value, error) {
	ident := uuid{
		UUID: relationUUID,
	}
	stmt, err := st.Prepare(`
SELECT &queryLife.*
FROM   relation t
JOIN   life l ON t.life_id = l.id
WHERE  t.uuid = $uuid.uuid
`, queryLife{}, ident)
	if err != nil {
		return "", errors.Capture(err)
	}

	var relationLife queryLife
	err = tx.Query(ctx, stmt, ident).Get(&relationLife)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", relationerrors.RelationNotFound
	} else if err != nil {
		return "", errors.Errorf("retrieving life for relation %q: %w", relationUUID, err)
	}

	return relationLife.Value, nil
}

// getRegularRelationUUIDByEndpointIdentifiers returns the relation UUID for a
// regular (non-peer) relation identified by two endpoint identifiers.
// It uses the v_relation_endpoint_identifier view to match both endpoints.
//
// It returns a [relationerrors.RelationNotFound] if no matching relation
// exists.
func (st *State) getRegularRelationUUIDByEndpointIdentifiers(
	ctx context.Context,
	tx *sqlair.TX,
	endpoint1, endpoint2 corerelation.EndpointIdentifier,
) (string, error) {
	// Use different types for each endpoint to satisfy sqlair's type system
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
		return "", errors.Capture(err)
	}

	var result uuid
	err = tx.Query(ctx, stmt, e1, e2).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", relationerrors.RelationNotFound
	}
	if err != nil {
		return "", errors.Capture(err)
	}

	return result.UUID, nil
}
