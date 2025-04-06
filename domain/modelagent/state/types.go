// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/life"
)

// dbAgentVersion represents the target agent version from the model table.
type dbAgentVersion struct {
	TargetAgentVersion string `db:"target_version"`
}

// architectureMap provides a way to exchange one architecture value
// for the other the database. i.e transfer from name to ID etc.
type architectureMap struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

// machineAgentVersion represents a record from the reported machine agent
// table.
type machineAgentVersion struct {
	MachineUUID    string `db:"machine_uuid"`
	Version        string `db:"version"`
	ArchitectureID int    `db:"architecture_id"`
}

// machineTargetAgentVersionInfo represents a record from the
// v_machine_target_agent_version view.
type machineTargetAgentVersionInfo struct {
	MachineUUID      string `db:"machine_uuid"`
	TargetVersion    string `db:"target_version"`
	ArchitectureName string `db:"architecture_name"`
}

type unitAgentVersionInfo struct {
	UnitUUID         coreunit.UUID `db:"unit_uuid"`
	TargetVersion    string        `db:"target_version"`
	ArchitectureName string        `db:"architecture_name"`
}

// machineLife represents the struct to be used for the life_id column within
// the sqlair statements in the machine domain.
type machineLife struct {
	UUID   string    `db:"uuid"`
	LifeID life.Life `db:"life_id"`
}

// machineName represents the single column of a machine that is the machines
// name.
type machineName struct {
	Name string `db:"name"`
}

// machineUUID represents the struct to be used for the machine_uuid column
// within the sqlair statements in the machine domain.
type machineUUID struct {
	UUID string `db:"uuid"`
}

// unitAgentVersion represents a record from the reported unit agent
// version table.
type unitAgentVersion struct {
	UnitUUID      string `db:"unit_uuid"`
	Version       string `db:"version"`
	ArchtectureID int    `db:"architecture_id"`
}

// unitName represents the single column of a unit that is the unit's name.
type unitName struct {
	Name string `db:"name"`
}

// unitUUID represents the uuid for a unit from the unit table.
type unitUUID struct {
	UnitUUID coreunit.UUID `db:"uuid"`
}
