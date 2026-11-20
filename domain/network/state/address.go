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
// value. When preferDefaultRoute is true, primary IPv4 addresses on the default
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
SELECT &SpaceAddress.*
FROM   v_ip_address_selection AS ipa
WHERE  ipa.net_node_uuid = $entityUUID.uuid
ORDER BY
    ipa.scope_rank,
    ipa.type_id,
    CASE
        WHEN $addressOrder.prefer_default_route AND ipa.is_secondary THEN 1
        ELSE 0
    END,
    CASE
        WHEN $addressOrder.prefer_default_route THEN ipa.default_route_rank
        ELSE 0
    END,
    ipa.origin_rank,
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

// GetNetNodePublicAddress returns the preferred public address of a net node,
// falling back to cloud-local scope. Within each scope, IPv4 precedes IPv6,
// primary addresses precede secondary addresses, and primary IPv4 addresses on
// the default-route device are preferred. Origin and value break ties. Other
// scopes are excluded.
//
// If no suitable address exists, it returns a zero address and no error.
func (st *State) GetNetNodePublicAddress(ctx context.Context, nodeUUID string) (corenetwork.SpaceAddress, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return corenetwork.SpaceAddress{}, errors.Capture(err)
	}

	ident := entityUUID{UUID: nodeUUID}
	stmt, err := st.Prepare(`
SELECT &SpaceAddress.*
FROM   v_ip_address_selection AS ipa
WHERE  ipa.net_node_uuid = $entityUUID.uuid
AND    ipa.scope_id IN (1 /* public */, 2 /* local-cloud */)
ORDER BY
    ipa.scope_rank,
    ipa.type_id,
    ipa.is_secondary,
    ipa.default_route_rank,
    ipa.origin_rank,
    ipa.address_value
LIMIT 1
`, SpaceAddress{}, entityUUID{})
	if err != nil {
		return corenetwork.SpaceAddress{}, errors.Capture(err)
	}

	var address SpaceAddress
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		address = SpaceAddress{}
		err := tx.Query(ctx, stmt, ident).Get(&address)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying public address: %w", err)
		}
		return nil
	})
	if err != nil {
		return corenetwork.SpaceAddress{}, errors.Capture(err)
	}
	if address.Value == "" {
		return corenetwork.SpaceAddress{}, nil
	}
	return encodeIPAddress(address)
}
