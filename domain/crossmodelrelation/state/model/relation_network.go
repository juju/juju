// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/errors"
)

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

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkRelationExists(ctx, tx, relationUUID); err != nil {
			return errors.Capture(err)
		}

		for _, cidr := range cidrs {
			ingress := relationNetworkIngress{
				RelationUUID: relationUUID,
				CIDR:         cidr,
			}
			if err := tx.Query(ctx, insertStmt, ingress).Run(); err != nil {
				return errors.Errorf("inserting relation network ingress for relation %q with CIDR %q: %w", relationUUID, cidr, err)
			}
		}
		return nil
	})

	return errors.Capture(err)
}

// GetRelationNetworkIngress retrieves all ingress network CIDRs for the
// specified relation.
// It returns a [relationerrors.RelationNotFound] if the provided relation does
// not exist.
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
		if err := st.checkRelationExists(ctx, tx, relationUUID); err != nil {
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

// checkRelationExists checks if a relation with the given UUID exists in the
// relation table.
// It returns a [relationerrors.RelationNotFound] if the relation does not
// exist.
func (st *State) checkRelationExists(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID string,
) error {
	query := `
SELECT COUNT(*) AS &countResult.count
FROM   relation 
WHERE  uuid = $uuid.uuid
`
	checkStmt, err := st.Prepare(query, countResult{}, uuid{})
	if err != nil {
		return errors.Capture(err)
	}

	var result countResult
	rel := uuid{UUID: relationUUID}
	err = tx.Query(ctx, checkStmt, rel).Get(&result)
	if err != nil {
		return errors.Errorf("checking relation exists: %w", err)
	}

	if result.Count == 0 {
		return relationerrors.RelationNotFound
	}

	return nil
}
