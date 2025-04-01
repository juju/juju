// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/juju/domain/life"

// dbAgentVersion represents the target agent version from the model table.
type dbAgentVersion struct {
	TargetAgentVersion string `db:"target_version"`
}

// machineName represents the single column of a machine that is the machines
// name.
type machineName struct {
	Name string `db:"name"`
}

// unitName represents the single column of a unit that is the unit's name.
type unitName struct {
	Name string `db:"name"`
}

// machineLife represents the struct to be used for the life_id column within
// the sqlair statements in the machine domain.
type machineLife struct {
	UUID   string    `db:"uuid"`
	LifeID life.Life `db:"life_id"`
}
