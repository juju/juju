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
	devices, err := st.deviceTypeDetails(ctx, tx)
	if err != nil {
		return nameToIDTable{}, errors.Capture(err)
	}
	ports, err := st.portTypeDetails(ctx, tx)
	if err != nil {
		return nameToIDTable{}, errors.Capture(err)
	}
	addrs, err := st.addrTypeDetails(ctx, tx)
	if err != nil {
		return nameToIDTable{}, errors.Capture(err)
	}
	config, err := st.addrConfigTypeDetails(ctx, tx)
	if err != nil {
		return nameToIDTable{}, errors.Capture(err)
	}
	origin, err := st.originTypeDetails(ctx, tx)
	if err != nil {
		return nameToIDTable{}, errors.Capture(err)
	}

	scope, err := st.scopeTypeDetails(ctx, tx)
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

func (st *State) deviceTypeDetails(ctx context.Context, tx *sqlair.TX) (map[network.LinkLayerDeviceType]int, error) {
	types, err := st.getTypeIDNames(ctx, tx, "link_layer_device_type")
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.SliceToMap(types, func(in typeIDName) (network.LinkLayerDeviceType, int) {
		return network.LinkLayerDeviceType(in.Name), in.ID
	}), nil
}

func (st *State) portTypeDetails(ctx context.Context, tx *sqlair.TX) (map[network.VirtualPortType]int, error) {
	types, err := st.getTypeIDNames(ctx, tx, "virtual_port_type")
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.SliceToMap(types, func(in typeIDName) (network.VirtualPortType, int) {
		return network.VirtualPortType(in.Name), in.ID
	}), nil
}

func (st *State) addrTypeDetails(ctx context.Context, tx *sqlair.TX) (map[network.AddressType]int, error) {
	types, err := st.getTypeIDNames(ctx, tx, "ip_address_type")
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.SliceToMap(types, func(in typeIDName) (network.AddressType, int) {
		return network.AddressType(in.Name), in.ID
	}), nil
}

func (st *State) addrConfigTypeDetails(ctx context.Context, tx *sqlair.TX) (map[network.AddressConfigType]int, error) {
	types, err := st.getTypeIDNames(ctx, tx, "ip_address_config_type")
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.SliceToMap(types, func(in typeIDName) (network.AddressConfigType, int) {
		return network.AddressConfigType(in.Name), in.ID
	}), nil
}

func (st *State) originTypeDetails(ctx context.Context, tx *sqlair.TX) (map[network.Origin]int, error) {
	types, err := st.getTypeIDNames(ctx, tx, "ip_address_origin")
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.SliceToMap(types, func(in typeIDName) (network.Origin, int) {
		return network.Origin(in.Name), in.ID
	}), nil
}

func (st *State) scopeTypeDetails(ctx context.Context, tx *sqlair.TX) (map[network.Scope]int, error) {
	types, err := st.getTypeIDNames(ctx, tx, "ip_address_scope")
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.SliceToMap(types, func(in typeIDName) (network.Scope, int) {
		return network.Scope(in.Name), in.ID
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
