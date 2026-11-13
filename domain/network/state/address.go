// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
)

// GetNetNodeAddresses fetches network node addresses associated with
// a given node UUID, together with the facts about their link-layer devices
// that are needed to identify the default route source address.
func (st *State) GetNetNodeAddresses(
	ctx context.Context, nodeUUID string,
) ([]corenetwork.DeviceAddress, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := entityUUID{UUID: nodeUUID}
	stmt, err := st.Prepare(`
SELECT    &deviceAddress.*
FROM      v_ip_address_with_names AS ipa
JOIN      link_layer_device AS lld ON ipa.device_uuid = lld.uuid
LEFT JOIN subnet AS sn ON ipa.subnet_uuid = sn.uuid
WHERE     ipa.net_node_uuid = $entityUUID.uuid
`, deviceAddress{}, entityUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var addresses []deviceAddress
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).GetAll(&addresses)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying addresses for net node %q: %w", nodeUUID, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return encodeDeviceAddresses(addresses)
}
