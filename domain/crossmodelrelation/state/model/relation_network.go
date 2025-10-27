// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"net"
	"strings"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/eventsource"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/environs/config"
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

// NamespacesForRelationEgressNetworksWatcher returns the namespaces of the
// tables needed for the relation egress networks watcher.
func (st *State) NamespacesForRelationEgressNetworksWatcher() (string, string, string) {
	return "relation_network_egress", "model_config", "ip_address"
}

// InitialWatchStatementForRelationEgressNetworks returns the initial query
// for watching relation egress networks. It returns the actual egress CIDRs
// if the relation exists, or an empty slice if no egress networks are
// configured.
func (st *State) InitialWatchStatementForRelationEgressNetworks(relationUUID string) eventsource.NamespaceQuery {
	return func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		db, err := st.DB(ctx)
		if err != nil {
			return nil, errors.Capture(err)
		}

		checkStmt, err := st.Prepare(`
SELECT &uuid.*
FROM   relation
WHERE  uuid = $uuid.uuid
`, uuid{})
		if err != nil {
			return nil, errors.Capture(err)
		}

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

// GetUnitAddressesForRelation returns all unit addresses for units that are
// part of the specified relation, grouped by unit UUID.
func (st *State) GetUnitAddressesForRelation(ctx context.Context, relationUUID string) (map[string]network.SpaceAddresses, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT u.uuid AS &unitAddress.unit_uuid,
       ua.address_value AS &unitAddress.address_value,
       ua.config_type_name AS &unitAddress.config_type_name,
       ua.type_name AS &unitAddress.type_name,
       ua.origin_name AS &unitAddress.origin_name,
       ua.scope_name AS &unitAddress.scope_name,
       ua.space_uuid AS &unitAddress.space_uuid,
       ua.cidr AS &unitAddress.cidr
FROM   relation_unit ru
JOIN   relation_endpoint re ON ru.relation_endpoint_uuid = re.uuid
JOIN   unit u ON ru.unit_uuid = u.uuid
JOIN   v_all_unit_address AS ua ON u.uuid = ua.unit_uuid
WHERE  re.relation_uuid = $uuid.uuid
`, unitAddress{}, uuid{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var addresses []unitAddress
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, uuid{UUID: relationUUID}).GetAll(&addresses)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Group addresses by unit UUID
	result := make(map[string]network.SpaceAddresses)
	for _, addr := range addresses {
		spaceAddr, err := encodeIPAddress(addr)
		if err != nil {
			return nil, errors.Capture(err)
		}
		result[addr.UnitUUID] = append(result[addr.UnitUUID], spaceAddr)
	}
	return result, nil
}

// GetModelEgressSubnets returns the egress-subnets configuration from model
// config.
func (st *State) GetModelEgressSubnets(ctx context.Context) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	egressSubnetsConfigKey := modelConfigKey{
		Key: config.EgressSubnets,
	}

	stmt, err := st.Prepare(`
SELECT &modelConfigValue.value
FROM   model_config
WHERE  key = $modelConfigKey.key
`, modelConfigValue{}, egressSubnetsConfigKey)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var configValue modelConfigValue
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, egressSubnetsConfigKey).Get(&configValue)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

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

// encodeIPAddress converts a unitAddress to a network.SpaceAddress.
// This is the same logic used in the network domain state layer.
func encodeIPAddress(address unitAddress) (network.SpaceAddress, error) {
	spaceUUID := network.AlphaSpaceId
	if address.SpaceUUID.Valid {
		spaceUUID = network.SpaceUUID(address.SpaceUUID.String)
	}
	// The saved address value is in the form 192.0.2.1/24,
	// parse the parts for the MachineAddress
	ipAddr, ipNet, err := net.ParseCIDR(address.Value)
	if err != nil {
		// Note: IP addresses from Kubernetes do not contain subnet
		// mask suffixes yet. Handle that scenario here. Eventually
		// an error should be returned instead.
		ipAddr = net.ParseIP(address.Value)
	}
	cidr := ""
	if ipNet != nil {
		cidr = ipNet.String()
	}
	// Prefer the subnet cidr if one exists.
	if address.CIDR.Valid {
		cidr = address.CIDR.String
	}
	return network.SpaceAddress{
		SpaceID: spaceUUID,
		Origin:  network.Origin(address.Origin),
		MachineAddress: network.MachineAddress{
			Value:      ipAddr.String(),
			CIDR:       cidr,
			Type:       network.AddressType(address.Type),
			Scope:      network.Scope(address.Scope),
			ConfigType: network.AddressConfigType(address.ConfigType),
		},
	}, nil
}
