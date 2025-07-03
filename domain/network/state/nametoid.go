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

type typeIDName struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

type nameToIDTable struct {
	DeviceMap     map[network.LinkLayerDeviceType]int
	PortMap       map[network.VirtualPortType]int
	AddrMap       map[network.AddressType]int
	AddrConfigMap map[network.AddressConfigType]int
	OriginMap     map[network.Origin]int
	ScopeMap      map[network.Scope]int
}

func (st *State) getNameToIDTable(ctx context.Context, tx *sqlair.TX) (nameToIDTable, error) {
	devices, err := typeNameToIDMap[network.LinkLayerDeviceType](ctx, st, tx, "link_layer_device_type")
	if err != nil {
		return nameToIDTable{}, errors.Capture(err)
	}
	ports, err := typeNameToIDMap[network.VirtualPortType](ctx, st, tx, "virtual_port_type")
	if err != nil {
		return nameToIDTable{}, errors.Capture(err)
	}
	addrs, err := typeNameToIDMap[network.AddressType](ctx, st, tx, "ip_address_type")
	if err != nil {
		return nameToIDTable{}, errors.Capture(err)
	}
	config, err := typeNameToIDMap[network.AddressConfigType](ctx, st, tx, "ip_address_config_type")
	if err != nil {
		return nameToIDTable{}, errors.Capture(err)
	}
	origin, err := typeNameToIDMap[network.Origin](ctx, st, tx, "ip_address_origin")
	if err != nil {
		return nameToIDTable{}, errors.Capture(err)
	}
	scope, err := typeNameToIDMap[network.Scope](ctx, st, tx, "ip_address_scope")
	if err != nil {
		return nameToIDTable{}, errors.Capture(err)
	}

	return nameToIDTable{
		DeviceMap:     devices,
		PortMap:       ports,
		AddrMap:       addrs,
		AddrConfigMap: config,
		OriginMap:     origin,
		ScopeMap:      scope,
	}, nil
}

func typeNameToIDMap[T ~string](ctx context.Context, st *State, tx *sqlair.TX, tableName string) (map[T]int, error) {
	types, err := st.getTypeIDNames(ctx, tx, tableName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.SliceToMap(types, func(in typeIDName) (T, int) {
		return T(in.Name), in.ID
	}), nil
}

func (st *State) getTypeIDNames(ctx context.Context, tx *sqlair.TX, table string) ([]typeIDName, error) {
	deviceTypeStmt, err := st.Prepare(fmt.Sprintf(`
SELECT &typeIDName.*
FROM %s
`, table), typeIDName{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var types []typeIDName
	if err := tx.Query(ctx, deviceTypeStmt).GetAll(&types); err != nil {
		return nil, errors.Errorf("querying link layer device types: %w", err)
	}
	return types, nil
}
