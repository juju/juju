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
// a given node UUID.
func (st *State) GetNetNodeAddresses(ctx context.Context, nodeUUID string) (corenetwork.SpaceAddresses, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var address []spaceAddress
	ident := entityUUID{UUID: nodeUUID}
	stmt, err := st.Prepare(`
SELECT    &spaceAddress.*
FROM      v_ip_address_with_names AS ipa
LEFT JOIN subnet AS sn ON ipa.subnet_uuid = sn.uuid
WHERE     net_node_uuid = $entityUUID.uuid
`, spaceAddress{}, entityUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).GetAll(&address)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying addresses for net node %q: %w", nodeUUID, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return encodeIpAddresses(address)
}
