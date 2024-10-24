// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stub

import "github.com/juju/juju/core/unit"

func encodeGroupedUnitsByMachine(grouped map[string][]unit.Name) map[machine]units {
	groupedUnitsByMachine := make(map[machine]units, len(grouped))
	for m, us := range grouped {
		groupedUnitsByMachine[machine{MachineName: m}] = units(us)
	}
	return groupedUnitsByMachine
}

type count struct {
	Count int `db:"count"`
}

type machine struct {
	MachineName string `db:"name"`
}

type units []unit.Name

type netNodeUUID struct {
	NetNodeUUID string `db:"net_node_uuid"`
}
