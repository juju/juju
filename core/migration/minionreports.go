// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

// MinionReports returns information about the migration minion
// reports received so far for a given migration phase.
type MinionReports struct {
	// ModelUUID holds the unique identifier for the model migration.
	MigrationId string

	// Phases indicates the migration phase the reports relate to.
	Phase Phase

	// SuccesCount indicates how many agents have successfully
	// completed the migration phase.
	SuccessCount int

	// UnknownCount indicates how many agents are yet to report
	// regarding the migration phase.
	UnknownCount int

	// SomeUnknownMachines holds the ids of some of the machines which
	// have not yet reported in.
	SomeUnknownMachines []string

	// SomeUnknownUnits holds the names of some of the units which
	// have not yet reported in.
	SomeUnknownUnits []string

	// SomeUnknownApplications holds the names of some of the applications which
	// have not yet reported in.
	SomeUnknownApplications []string

	// FailedMachines holds the ids of machines which have failed to
	// complete the migration phase.
	FailedMachines []string

	// FailedUnits holds the names of units which have failed to
	// complete the migration phase.
	FailedUnits []string

	// FailedApplications holds the names of applications which have failed to
	// complete the migration phase.
	FailedApplications []string
}

// IsZero returns true if the MinionReports instance hasn't been set.
func (r *MinionReports) IsZero() bool {
	return r.MigrationId == "" && r.Phase == UNKNOWN
}
