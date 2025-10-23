// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"strings"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher/eventsource"
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

// GetRelationNetworkEgress retrieves all egress network CIDRs for the
// specified relation.
//
// It returns a [relationerrors.RelationNotFound] if the provided relation does
// not exist.
func (st *State) GetRelationNetworkEgress(ctx context.Context, relationUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	selectStmt, err := st.Prepare(`
SELECT &cidr.*
FROM   relation_network_egress
WHERE  relation_uuid = $uuid.uuid
`, cidr{}, uuid{})
	if err != nil {
		return nil, errors.Errorf("preparing select relation network egress query: %w", err)
	}

	var cidrs []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// We ignore the returned life value here, we just want to check
		// that the relation exists and return its egress networks even if
		// not alive.
		if _, err := st.getRelationLife(ctx, tx, relationUUID); err != nil {
			return errors.Capture(err)
		}

		var results []cidr
		if err := tx.Query(ctx, selectStmt, uuid{UUID: relationUUID}).GetAll(&results); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("retrieving relation network egress for relation %q: %w", relationUUID, err)
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

// NamespaceForRelationEgressNetworksWatcher returns the namespace of the
// relation_network_egress table, used for the watcher.
func (st *State) NamespaceForRelationEgressNetworksWatcher() string {
	return "relation_network_egress"
}

// InitialWatchStatementForRelationEgressNetworks returns the initial query
// for watching relation egress networks. It returns the actual egress CIDRs
// if the relation exists, or an empty slice if no egress networks are configured.
func (st *State) InitialWatchStatementForRelationEgressNetworks(relationUUID string) eventsource.NamespaceQuery {
	return func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		db, err := st.DB(ctx)
		if err != nil {
			return nil, errors.Capture(err)
		}

		// First verify the relation exists
		checkStmt, err := st.Prepare(`
SELECT &uuid.*
FROM   relation
WHERE  uuid = $uuid.uuid
`, uuid{})
		if err != nil {
			return nil, errors.Capture(err)
		}

		// Then get the egress CIDRs
		selectStmt, err := st.Prepare(`
SELECT &cidr.*
FROM   relation_network_egress
WHERE  relation_uuid = $uuid.uuid
`, cidr{}, uuid{})
		if err != nil {
			return nil, errors.Capture(err)
		}

		var cidrs []string
		err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			var result uuid
			err := tx.Query(ctx, checkStmt, uuid{UUID: relationUUID}).Get(&result)
			if errors.Is(err, sqlair.ErrNoRows) {
				return relationerrors.RelationNotFound
			}
			if err != nil {
				return err
			}

			var results []cidr
			if err := tx.Query(ctx, selectStmt, uuid{UUID: relationUUID}).GetAll(&results); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("retrieving relation network egress for relation %q: %w", relationUUID, err)
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
}

// GetNetNodeUUIDsForRelation returns all net_node_uuids for units that are
// part of the specified relation.
func (st *State) GetNetNodeUUIDsForRelation(ctx context.Context, relationUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT DISTINCT &netNodeUUID.*
FROM   relation_unit ru
JOIN   relation_endpoint re ON ru.relation_endpoint_uuid = re.uuid
JOIN   unit u ON ru.unit_uuid = u.uuid
WHERE  re.relation_uuid = $uuid.uuid
`, netNodeUUID{}, uuid{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var netNodeUUIDs []netNodeUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, uuid{UUID: relationUUID}).GetAll(&netNodeUUIDs)
	})
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	result := make([]string, len(netNodeUUIDs))
	for i, n := range netNodeUUIDs {
		result[i] = n.NetNodeUUID
	}
	return result, nil
}

// GetUnitAddressesForRelation returns all unit addresses for units that are
// part of the specified relation. Only public addresses are returned.
func (st *State) GetUnitAddressesForRelation(ctx context.Context, relationUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT DISTINCT ia.address_value AS &ipAddress.address_value
FROM   relation_unit ru
JOIN   relation_endpoint re ON ru.relation_endpoint_uuid = re.uuid
JOIN   unit u ON ru.unit_uuid = u.uuid
JOIN   ip_address ia ON ia.net_node_uuid = u.net_node_uuid
WHERE  re.relation_uuid = $uuid.uuid
  AND  ia.scope_id = 1  -- Public addresses only (scope_id 1 = public)
  AND  ia.address_value IS NOT NULL
  AND  ia.address_value != ''
`, ipAddress{}, uuid{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var addresses []ipAddress
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, uuid{UUID: relationUUID}).GetAll(&addresses)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No addresses found, return empty slice
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make([]string, len(addresses))
	for i, addr := range addresses {
		result[i] = addr.AddressValue
	}
	return result, nil
}

// GetModelEgressSubnets returns the egress-subnets configuration from model config.
func (st *State) GetModelEgressSubnets(ctx context.Context) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &modelConfigValue.value
FROM   model_config
WHERE  key = 'egress-subnets'
`, modelConfigValue{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var configValue modelConfigValue
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&configValue)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No egress-subnets configured, return empty slice
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Parse the comma-separated list of CIDRs
	if configValue.Value == "" {
		return []string{}, nil
	}

	cidrs := strings.Split(configValue.Value, ",")
	result := make([]string, 0, len(cidrs))
	for _, cidr := range cidrs {
		trimmed := strings.TrimSpace(cidr)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result, nil
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
