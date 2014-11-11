package juju

// temporary types to break the state <> apiserver dependency loop

// RelationUnitsChange holds notifications of units entering and leaving the
// scope of a RelationUnit, and changes to the settings of those units known
// to have entered.
//
// When remote units first enter scope and then when their settings
// change, the changes will be noted in the Changed field, which holds
// the unit settings for every such unit, indexed by the unit id.
//
// When remote units leave scope, their ids will be noted in the
// Departed field, and no further events will be sent for those units.
type RelationUnitsChange struct {
	Changed  map[string]UnitSettings
	Departed []string
}

// UnitSettings holds information about a service unit's settings
// within a relation.
type UnitSettings struct {
	Version int64
}

// MachineJob values define responsibilities that machines may be
// expected to fulfil.
type MachineJob string

const (
	JobHostUnits        MachineJob = "JobHostUnits"
	JobManageEnviron    MachineJob = "JobManageEnviron"
	JobManageNetworking MachineJob = "JobManageNetworking"

	// Deprecated in 1.18
	JobManageStateDeprecated MachineJob = "JobManageState"
)

// NeedsState returns true if the job requires a state connection.
func (job MachineJob) NeedsState() bool {
	return job == JobManageEnviron
}

// AnyJobNeedsState returns true if any of the provided jobs
// require a state connection.
func AnyJobNeedsState(jobs ...MachineJob) bool {
	for _, j := range jobs {
		if j.NeedsState() {
			return true
		}
	}
	return false
}
