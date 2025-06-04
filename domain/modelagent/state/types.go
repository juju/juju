// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/life"
)

// agentVersionStream represents the stream id that is in use by the record
// in the agent_version table.
type agentVersionStream struct {
	StreamID int `db:"stream_id"`
}

// agentVersionTarget represents the target agent version column from the
// agent_version table.
type agentVersionTarget struct {
	TargetVersion string `db:"target_version"`
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

// machineAgentVersionInfo represents a record from the v_machine_agent_version
// view.
type machineAgentVersionInfo struct {
	MachineUUID  string `db:"machine_uuid"`
	Version      string `db:"version"`
	Architecture string `db:"architecture_name"`
}

// machineBaseValues represents a set of base values associated with a machine.
type machineBaseValues []string

// machineCount represents the result of counting the number of machines that
// match a sql expression.
type machineCount struct {
	// Count represents the number of machines that have been counted for a
	// query.
	Count int `db:"count"`
}

// setAgentVersionTarget represents the set of update values required for
// setting the model's target agent version.
type setAgentVersionTarget struct {
	TargetVersion string `db:"target_version"`
}

// setAgentVersionTargetStream represents the set of update values required for
// setting the model's target agent version and stream.
type setAgentVersionTargetStream struct {
	StreamID      int    `db:"stream_id"`
	TargetVersion string `db:"target_version"`
}

// rowCount is a quick type that can be used with aggregation queries to store
// the value of a count().
type rowCount struct {
	Count int `db:"count"`
}

// machineAgentBinaryMetadata represents information about a machine and the
// agent binaries that it is running.
type machineAgentBinaryMetadata struct {
	MachineName  string         `db:"name"`
	Version      string         `db:"version"`
	Architecture string         `db:"architecture_name"`
	Size         sql.NullInt64  `db:"size"`
	SHA256       sql.NullString `db:"sha_256"`
	SHA384       sql.NullString `db:"sha_384"`
}

// machineTargetAgentVersionInfo represents a record from the
// v_machine_target_agent_version view.
type machineTargetAgentVersionInfo struct {
	MachineUUID      string `db:"machine_uuid"`
	TargetVersion    string `db:"target_version"`
	ArchitectureName string `db:"architecture_name"`
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

// machineUUIDRef represents a machine uuid reference to the machine table.
type machineUUIDRef struct {
	UUID string `db:"machine_uuid"`
}

// unitAgentBinaryMetadata represents information about a unit and the
// agent binaries that it is running.
type unitAgentBinaryMetadata struct {
	UnitName     string         `db:"name"`
	Version      string         `db:"version"`
	Architecture string         `db:"architecture_name"`
	Size         sql.NullInt64  `db:"size"`
	SHA256       sql.NullString `db:"sha_256"`
	SHA384       sql.NullString `db:"sha_384"`
}

// unitAgentVersion represents a record from the reported unit agent
// version table.
type unitAgentVersion struct {
	UnitUUID      string `db:"unit_uuid"`
	Version       string `db:"version"`
	ArchtectureID int    `db:"architecture_id"`
}

// unitAgentVersionInfo represents a record from the unit agent version table.
type unitAgentVersionInfo struct {
	UnitUUID         string `db:"unit_uuid"`
	Version          string `db:"version"`
	ArchitectureName string `db:"name"`
}

// unitName represents the single column of a unit that is the unit's name.
type unitName struct {
	Name string `db:"name"`
}

// unitTargetAgentVersionInfo represents a record from the
// v_unit_target_agent_version view.
type unitTargetAgentVersionInfo struct {
	UnitUUID         coreunit.UUID `db:"unit_uuid"`
	TargetVersion    string        `db:"target_version"`
	ArchitectureName string        `db:"architecture_name"`
}

// unitUUID represents the uuid for a unit from the unit table.
type unitUUID struct {
	UnitUUID coreunit.UUID `db:"uuid"`
}

// unitUUIDRef represents a unit uuid reference to the unit table.
type unitUUIDRef struct {
	UUID coreunit.UUID `db:"unit_uuid"`
}
