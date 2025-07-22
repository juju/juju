// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
)

type netConfigLookups struct {
	deviceType      map[network.LinkLayerDeviceType]int
	virtualPortType map[network.VirtualPortType]int
	addrType        map[network.AddressType]int
	addrConfigType  map[network.AddressConfigType]int
	origin          map[network.Origin]int
	scope           map[network.Scope]int
}

func (st *State) getNetConfigLookups(ctx context.Context, tx *sqlair.TX) (netConfigLookups, error) {
	var err error
	lookups := netConfigLookups{}

	lookups.deviceType, err = getLookupNameToID[network.LinkLayerDeviceType](ctx, st, tx, "link_layer_device_type")
	if err != nil {
		return lookups, errors.Capture(err)
	}
	lookups.virtualPortType, err = getLookupNameToID[network.VirtualPortType](ctx, st, tx, "virtual_port_type")
	if err != nil {
		return lookups, errors.Capture(err)
	}
	lookups.addrType, err = getLookupNameToID[network.AddressType](ctx, st, tx, "ip_address_type")
	if err != nil {
		return lookups, errors.Capture(err)
	}
	lookups.addrConfigType, err = getLookupNameToID[network.AddressConfigType](ctx, st, tx, "ip_address_config_type")
	if err != nil {
		return lookups, errors.Capture(err)
	}
	lookups.origin, err = getLookupNameToID[network.Origin](ctx, st, tx, "ip_address_origin")
	if err != nil {
		return lookups, errors.Capture(err)
	}
	lookups.scope, err = getLookupNameToID[network.Scope](ctx, st, tx, "ip_address_scope")
	if err != nil {
		return lookups, errors.Capture(err)
	}

	return lookups, nil
}

// lookupRow represents an ID and name from a lookup table.
type lookupRow struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

func getLookupNameToID[T ~string](ctx context.Context, st *State, tx *sqlair.TX, tableName string) (map[T]int, error) {
	types, err := st.getLookup(ctx, tx, tableName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.SliceToMap(types, func(in lookupRow) (T, int) {
		return T(in.Name), in.ID
	}), nil
}

func (st *State) getLookup(ctx context.Context, tx *sqlair.TX, table string) ([]lookupRow, error) {
	deviceTypeStmt, err := st.Prepare(fmt.Sprintf("SELECT &lookupRow.* FROM %s", table), lookupRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var types []lookupRow
	if err := tx.Query(ctx, deviceTypeStmt).GetAll(&types); err != nil {
		return nil, errors.Errorf("querying link layer device types: %w", err)
	}
	return types, nil
}
