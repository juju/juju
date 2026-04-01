// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/network"
	networkinternal "github.com/juju/juju/domain/network/internal"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
)

// GetUnitRelationEndpointName retrieves the endpoint name used by the
// specified unit in the specified relation.
func (st *State) GetUnitRelationEndpointName(
	ctx context.Context,
	unitUUID, relationUUID string,
) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	type relationUnit struct {
		RelationUUID string `db:"relation_uuid"`
		UnitUUID     string `db:"unit_uuid"`
	}
	type endpointName struct {
		Name string `db:"name"`
	}

	arg := relationUnit{
		RelationUUID: relationUUID,
		UnitUUID:     unitUUID,
	}
	stmt, err := st.Prepare(`
SELECT cr.name AS &endpointName.name
FROM   unit AS u
JOIN   application_endpoint AS ae ON ae.application_uuid = u.application_uuid
JOIN   relation_endpoint AS re
       ON re.endpoint_uuid = ae.uuid
JOIN   charm_relation AS cr ON cr.uuid = ae.charm_relation_uuid
WHERE  re.relation_uuid = $relationUnit.relation_uuid
AND    u.uuid = $relationUnit.unit_uuid
`, endpointName{}, relationUnit{})
	if err != nil {
		return "", errors.Errorf("preparing relation endpoint name statement: %w", err)
	}

	var endpoint endpointName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, arg).Get(&endpoint)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("relation not found").Add(relationerrors.RelationNotFound)
		} else if err != nil {
			return errors.Errorf("querying relation endpoint name: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return endpoint.Name, nil
}

// GetUnitEndpointNetworkInfo retrieves raw unit addresses and selected ingress
// addresses for the specified endpoints.
func (st *State) GetUnitEndpointNetworkInfo(
	ctx context.Context,
	unitUUID string,
	endpointNames []string,
) ([]networkinternal.EndpointNetworkInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type names []string
	type InSpaceAddress spaceAddress
	type endpointNetworkInfoRow struct {
		EndpointName string `db:"endpoint_name"`
		InSpaceAddress
		Device         string         `db:"name"`
		MAC            string         `db:"mac_address"`
		DeviceType     string         `db:"device_type"`
		IngressAddress sql.NullString `db:"ingress_address_value"`
	}

	ident := entityUUID{UUID: unitUUID}
	endpoints := names(endpointNames)
	var rows []endpointNetworkInfoRow
	stmt, err := st.Prepare(`
WITH endpoint_binding AS (
    SELECT ae.application_uuid,
           cr.name,
           ae.space_uuid
    FROM   application_endpoint AS ae
    JOIN   charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
    UNION ALL
    SELECT aee.application_uuid,
           ceb.name,
           aee.space_uuid
    FROM   application_extra_endpoint AS aee
    JOIN   charm_extra_binding AS ceb
           ON aee.charm_extra_binding_uuid = ceb.uuid
),
endpoint_space AS (
    SELECT eb.name AS endpoint_name,
           IFNULL(eb.space_uuid, a.space_uuid) AS space_uuid
    FROM   endpoint_binding AS eb
    JOIN   unit AS u ON eb.application_uuid = u.application_uuid
    JOIN   application AS a ON u.application_uuid = a.uuid
    WHERE  u.uuid = $entityUUID.uuid
    AND    eb.name IN ($names[:])
),
ingress_candidate AS (
    SELECT es.endpoint_name,
           urn.address_value,
           urn.device_uuid,
           urn.scope_rank,
           urn.origin_id,
           urn.is_secondary,
           urn.device_type_id
    FROM   endpoint_space AS es
    JOIN   v_unit_relation_network AS urn
           ON urn.unit_uuid = $entityUUID.uuid
          AND urn.space_uuid = es.space_uuid
),
best_rank AS (
    SELECT endpoint_name, MIN(scope_rank) AS scope_rank
    FROM   ingress_candidate
    WHERE  scope_rank IS NOT NULL
    GROUP BY endpoint_name
),
selected_ingress AS (
    SELECT ic.endpoint_name,
           ic.address_value,
           ic.device_uuid,
           ic.origin_id,
           ic.is_secondary,
           ic.device_type_id
    FROM   ingress_candidate AS ic
    JOIN   best_rank USING (endpoint_name, scope_rank)
),
lld AS (
    SELECT lld.uuid,
           lld.name,
           lld.mac_address,
           lldt.name AS device_type
    FROM   link_layer_device AS lld
    JOIN   link_layer_device_type AS lldt ON lld.device_type_id = lldt.id
),
endpoint_address AS (
    SELECT es.endpoint_name,
           urn.address_value,
           urn.config_type_name,
           urn.type_name,
           urn.origin_name,
           urn.scope_name,
           urn.device_uuid,
           urn.space_uuid,
           urn.cidr,
           lld.name,
           lld.mac_address,
           lld.device_type,
           si.address_value AS ingress_address_value,
           si.origin_id AS ingress_origin_id,
           si.is_secondary AS ingress_is_secondary,
           si.device_type_id AS ingress_device_type_id
    FROM   endpoint_space AS es
    JOIN   v_unit_relation_network AS urn
           ON urn.unit_uuid = $entityUUID.uuid
          AND urn.space_uuid = es.space_uuid
    JOIN   lld ON urn.device_uuid = lld.uuid
    LEFT JOIN selected_ingress AS si
           ON si.endpoint_name = es.endpoint_name
          AND si.device_uuid = urn.device_uuid
          AND si.address_value = urn.address_value
)
SELECT &endpointNetworkInfoRow.*
FROM   endpoint_address
ORDER BY endpoint_name,
         CASE WHEN ingress_address_value IS NULL THEN 1 ELSE 0 END,
         ingress_device_type_id,
         ingress_is_secondary, /* primary (0) before secondary (1) */
         ingress_origin_id DESC, /* provider (1) before machine (0) */
         address_value,
         name
`, endpointNetworkInfoRow{}, entityUUID{}, endpoints)
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident, endpoints).GetAll(&rows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"querying endpoint networks: %w", err,
			)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	infosByEndpoint := transform.SliceToMap(
		endpointNames,
		func(name string) (string, networkinternal.EndpointNetworkInfo) {
			return name, networkinternal.EndpointNetworkInfo{EndpointName: name}
		},
	)
	for _, row := range rows {
		info := infosByEndpoint[row.EndpointName]

		encodedIP, err := encodeIPAddress(spaceAddress(row.InSpaceAddress))
		if err != nil {
			return nil, errors.Capture(err)
		}
		info.Addresses = append(info.Addresses, networkinternal.UnitAddress{
			SpaceAddress: encodedIP,
			DeviceName:   row.Device,
			MACAddress:   row.MAC,
			DeviceType:   network.LinkLayerDeviceType(row.DeviceType),
		})
		if row.IngressAddress.Valid {
			info.IngressAddresses = append(info.IngressAddresses, row.IngressAddress.String)
		}
		infosByEndpoint[row.EndpointName] = info
	}

	infos, _ := transform.SliceOrErr(
		endpointNames,
		func(name string) (networkinternal.EndpointNetworkInfo, error) {
			return infosByEndpoint[name], nil
		},
	)

	return infos, nil
}

// GetUnitNetworkInfo retrieves raw unit addresses and selected ingress
// addresses for the specified unit when provider networking is not supported.
func (st *State) GetUnitNetworkInfo(
	ctx context.Context,
	unitUUID string,
) (networkinternal.UnitNetworkInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return networkinternal.UnitNetworkInfo{}, errors.Capture(err)
	}

	type InSpaceAddress spaceAddress
	type unitNetworkInfoRow struct {
		InSpaceAddress
		Device         string         `db:"name"`
		MAC            string         `db:"mac_address"`
		DeviceType     string         `db:"device_type"`
		IngressAddress sql.NullString `db:"ingress_address_value"`
	}

	var rows []unitNetworkInfoRow
	ident := entityUUID{UUID: unitUUID}
	stmt, err := st.Prepare(`
WITH candidate AS (
    SELECT urn.address_value,
           urn.device_uuid,
           urn.scope_rank,
           urn.origin_id,
           urn.is_secondary,
           urn.device_type_id
    FROM   v_unit_relation_network AS urn
    WHERE  urn.unit_uuid = $entityUUID.uuid
),
best_rank AS (
    SELECT MIN(scope_rank) AS scope_rank
    FROM   candidate
    WHERE  scope_rank IS NOT NULL
),
selected_ingress AS (
    SELECT c.address_value,
           c.device_uuid,
           c.origin_id,
           c.is_secondary,
           c.device_type_id
    FROM   candidate AS c
    JOIN   best_rank USING (scope_rank)
),
lld AS (
    SELECT lld.uuid,
           lld.name,
           lld.mac_address,
           lldt.name AS device_type
    FROM   link_layer_device AS lld
    JOIN   link_layer_device_type AS lldt ON lld.device_type_id = lldt.id
),
unit_address AS (
    SELECT urn.address_value,
           urn.config_type_name,
           urn.type_name,
           urn.origin_name,
           urn.scope_name,
           urn.device_uuid,
           urn.space_uuid,
           urn.cidr,
           lld.name,
           lld.mac_address,
           lld.device_type,
           si.address_value AS ingress_address_value,
           si.origin_id AS ingress_origin_id,
           si.is_secondary AS ingress_is_secondary,
           si.device_type_id AS ingress_device_type_id
    FROM   v_unit_relation_network AS urn
    JOIN   lld ON urn.device_uuid = lld.uuid
    LEFT JOIN selected_ingress AS si
           ON si.device_uuid = urn.device_uuid
          AND si.address_value = urn.address_value
    WHERE  urn.unit_uuid = $entityUUID.uuid
)
SELECT &unitNetworkInfoRow.*
FROM   unit_address
ORDER BY CASE WHEN ingress_address_value IS NULL THEN 1 ELSE 0 END,
         ingress_device_type_id,
         ingress_is_secondary, /* primary (0) before secondary (1) */
         ingress_origin_id DESC, /* provider (1) before machine (0) */
         address_value
`, unitNetworkInfoRow{}, entityUUID{})
	if err != nil {
		return networkinternal.UnitNetworkInfo{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).GetAll(&rows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying unit network: %w", err)
		}
		return nil
	})
	if err != nil {
		return networkinternal.UnitNetworkInfo{}, errors.Capture(err)
	}

	info := networkinternal.UnitNetworkInfo{}
	for _, row := range rows {
		encodedIP, err := encodeIPAddress(spaceAddress(row.InSpaceAddress))
		if err != nil {
			return networkinternal.UnitNetworkInfo{}, errors.Capture(err)
		}
		info.Addresses = append(info.Addresses, networkinternal.UnitAddress{
			SpaceAddress: encodedIP,
			DeviceName:   row.Device,
			MACAddress:   row.MAC,
			DeviceType:   network.LinkLayerDeviceType(row.DeviceType),
		})
		if row.IngressAddress.Valid {
			info.IngressAddresses = append(info.IngressAddresses, row.IngressAddress.String)
		}
	}

	return info, nil
}

// GetModelEgressSubnets retrieves the egress-subnets configuration from model
// config.
func (st *State) GetModelEgressSubnets(ctx context.Context) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type modelConfigEntry struct {
		Key   string `db:"key"`
		Value string `db:"value"`
	}

	egressSubnetsConfig := modelConfigEntry{Key: config.EgressSubnets}
	stmt, err := st.Prepare(`
SELECT &modelConfigEntry.value
FROM   model_config
WHERE  key = $modelConfigEntry.key
`, modelConfigEntry{})
	if err != nil {
		return nil, errors.Errorf("preparing model egress subnets statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, egressSubnetsConfig).Get(&egressSubnetsConfig)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	if egressSubnetsConfig.Value == "" {
		return []string{}, nil
	}

	cidrs := strings.Split(egressSubnetsConfig.Value, ",")
	result := make([]string, 0, len(cidrs))
	for _, cidr := range cidrs {
		trimmed := strings.TrimSpace(cidr)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result, nil
}

// GetRelationEgressSubnets retrieves the egress subnets for the specified
// relation.
func (st *State) GetRelationEgressSubnets(ctx context.Context, relationUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := entityUUID{UUID: relationUUID}
	stmt, err := st.Prepare(`
SELECT DISTINCT rne.cidr AS &egressCIDR.cidr
FROM   relation_network_egress AS rne
WHERE  rne.relation_uuid = $entityUUID.uuid
`, egressCIDR{}, entityUUID{})
	if err != nil {
		return nil, errors.Errorf("preparing select relation egress subnets statement: %w", err)
	}

	var cidrs []egressCIDR
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).GetAll(&cidrs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying relation egress subnets: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	if len(cidrs) == 0 {
		return nil, nil
	}
	return transform.Slice(cidrs, func(c egressCIDR) string { return c.CIDR }), nil
}

// GetUnitEgressSubnets retrieves the egress subnets for the specified unit.
func (st *State) GetUnitEgressSubnets(ctx context.Context, unitUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := entityUUID{UUID: unitUUID}
	stmt, err := st.Prepare(`
SELECT DISTINCT rne.cidr AS &egressCIDR.cidr
FROM   relation_network_egress AS rne
JOIN   relation AS r ON rne.relation_uuid = r.uuid
JOIN   relation_endpoint AS re ON r.uuid = re.relation_uuid
JOIN   relation_unit AS ru ON re.uuid = ru.relation_endpoint_uuid
WHERE  ru.unit_uuid = $entityUUID.uuid
`, egressCIDR{}, entityUUID{})
	if err != nil {
		return nil, errors.Errorf("preparing select egress subnets statement: %w", err)
	}

	var cidrs []egressCIDR
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).GetAll(&cidrs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying egress subnets for unit %q: %w", unitUUID, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	if len(cidrs) == 0 {
		return nil, nil
	}
	return transform.Slice(cidrs, func(c egressCIDR) string { return c.CIDR }), nil
}

// IsCaasUnit determines if a unit identified by the given UUID is tied to a
// Kubernetes service.
func (st *State) IsCaasUnit(ctx context.Context, uuid string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	unitUUID := entityUUID{UUID: uuid}

	stmt, err := st.Prepare(`
SELECT u.uuid AS &entityUUID.uuid
FROM   k8s_service AS ks
JOIN   unit AS u ON ks.application_uuid = u.application_uuid
WHERE  u.uuid = $entityUUID.uuid`, entityUUID{})
	if err != nil {
		return false, errors.Errorf("preparing query: %w", err)
	}

	var result bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, unitUUID).Get(&unitUUID)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying k8s_service: %w", err)
		}
		result = err == nil
		return nil
	})

	return result, err
}
