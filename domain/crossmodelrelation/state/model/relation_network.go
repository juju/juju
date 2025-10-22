// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/life"
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
