// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
)

// GetNetNodeAddresses fetches network node addresses associated with a given
// node UUID. Addresses are ordered by public scope, address type, origin and
// value. When preferDefaultRoute is true, primary addresses on the default
// route device are preferred within their scope and type. If several devices
// carry a default route, or a device has several primary addresses, origin
// and value decide between them because no route metric is stored.
func (st *State) GetNetNodeAddresses(
	ctx context.Context, nodeUUID string, preferDefaultRoute bool,
) (corenetwork.SpaceAddresses, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := entityUUID{UUID: nodeUUID}
	order := addressOrder{PreferDefaultRoute: preferDefaultRoute}
	stmt, err := st.Prepare(`
SELECT ipa.address_value AS &SpaceAddress.address_value,
       iact.name AS &SpaceAddress.config_type_name,
       iat.name AS &SpaceAddress.type_name,
       iao.name AS &SpaceAddress.origin_name,
       ias.name AS &SpaceAddress.scope_name,
       ipa.device_uuid AS &SpaceAddress.device_uuid,
       sn.space_uuid AS &SpaceAddress.space_uuid,
       sn.cidr AS &SpaceAddress.cidr
FROM      ip_address AS ipa
JOIN      link_layer_device AS lld ON ipa.device_uuid = lld.uuid
JOIN      ip_address_config_type AS iact ON ipa.config_type_id = iact.id
JOIN      ip_address_type AS iat ON ipa.type_id = iat.id
JOIN      ip_address_origin AS iao ON ipa.origin_id = iao.id
JOIN      ip_address_scope AS ias ON ipa.scope_id = ias.id
LEFT JOIN subnet AS sn ON ipa.subnet_uuid = sn.uuid
WHERE     ipa.net_node_uuid = $entityUUID.uuid
ORDER BY
    CASE
        WHEN ipa.scope_id = 1 THEN 0
        WHEN ipa.scope_id = 2 THEN 1
        WHEN ipa.scope_id = 0 THEN 2
        WHEN ipa.scope_id = 3 THEN 3
        WHEN ipa.scope_id = 4 THEN 4
        ELSE 5
    END,
    ipa.type_id,
    CASE
        WHEN $addressOrder.prefer_default_route AND ipa.is_secondary THEN 1
        ELSE 0
    END,
    CASE
        WHEN $addressOrder.prefer_default_route AND lld.is_default_gateway THEN 0
        ELSE 1
    END,
    CASE
        WHEN ipa.origin_id = 1 THEN 0
        WHEN ipa.origin_id = 0 THEN 1
        ELSE 2
    END,
    ipa.address_value
`, SpaceAddress{}, entityUUID{}, addressOrder{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var address []SpaceAddress
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident, order).GetAll(&address)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying addresses for net node %q: %w", nodeUUID, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return encodeIPAddresses(address)
}
