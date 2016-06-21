// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import "github.com/juju/version"

// MigrationStatus returns the details for a migration as needed by
// the migration master worker.
type MigrationStatus struct {
	// ModelUUID holds the UUID of the model being migrated.
	ModelUUID string

	// Attempt specifies the migration attempt number. This is
	// incremeted for each attempt to migrate a model.
	Attempt int

	// Phases indicates the current migration phase.
	Phase Phase

	// TargetInfo contains the details of how to connect to the target
	// controller.
	TargetInfo TargetInfo
}

// SerializedModel wraps a buffer contain a serialised Juju model as
// well as containing metadata about the charms and tools used by the
// model.
type SerializedModel struct {
	// Bytes contains the serialized data for the model.
	Bytes []byte

	// Charms lists the charm URLs in use in the model.
	Charms []string

	// Tools lists the tools versions in use with the model along with
	// their URIs. The URIs can be used to download the tools from the
	// source controller.
	Tools map[version.Binary]string // version -> tools URI
}

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

	// FailedMachines holds the ids of machines which have failed to
	// complete the migration phase.
	FailedMachines []string

	// FailedUnits holds the names of units which have failed to
	// complete the migration phase.
	FailedUnits []string
}
