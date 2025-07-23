// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
)

// netConfigLookups holds the mappings from names to IDs for various network
// lookup tables. These are populated up-front to simplify database access in
// the course of network configuration updates, while also ensuring that we
// take a data-driven approach to values from fixed sets.
type netConfigLookups struct {
	deviceType      map[network.LinkLayerDeviceType]int
	virtualPortType map[network.VirtualPortType]int
	addrType        map[network.AddressType]int
	addrConfigType  map[network.AddressConfigType]int
	origin          map[network.Origin]int
	scope           map[network.Scope]int
}

// getNetConfigLookups queries the lookup table data and returns a populated
// [netConfigLookups] struct.
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

// getLookupNameToID retrieves a mapping of names to IDs
// for the lookup table with the input name.
// There is a naive guard against SQL injection by checking
// for spaces in the table name.
func getLookupNameToID[T ~string](ctx context.Context, st *State, tx *sqlair.TX, tableName string) (map[T]int, error) {
	if strings.Contains(tableName, " ") {
		return nil, errors.Errorf("invalid table name: %q", tableName)
	}

	deviceTypeStmt, err := st.Prepare(fmt.Sprintf("SELECT &lookupRow.* FROM %s", tableName), lookupRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var types []lookupRow
	if err := tx.Query(ctx, deviceTypeStmt).GetAll(&types); err != nil {
		return nil, errors.Errorf("querying link layer device types: %w", err)
	}

	return transform.SliceToMap(types, func(in lookupRow) (T, int) { return T(in.Name), in.ID }), nil
}
