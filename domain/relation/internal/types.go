// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	domainapplication "github.com/juju/juju/domain/application"
	domainnetwork "github.com/juju/juju/domain/network"
	domainrelation "github.com/juju/juju/domain/relation"
	domainstorage "github.com/juju/juju/domain/storage"
)

// WatcherRelationUnitsData contains data returned by the
// WatcherRelationUnitsData state method. This ensures that the
// order of the strings cannot be misinterpreted.
type WatcherRelationUnitsData struct {
	RelationEndpointUUID      string
	RelationUnitNS            string
	ApplicationSettingsHashNS string
	UnitSettingsHashNS        string
}

// RelationLifeSuspendedStatus describes the life and suspended status
// of a relation. Endpoints are included to create a relation key for the
// domain version of this structure.
type RelationLifeSuspendedStatus struct {
	// Life is the life of the relation.
	Life life.Value
	// Suspended is the suspended status of the relation.
	Suspended bool
	// SuspendedReason is an optional message to explain why suspended is true.
	SuspendedReason string
	// Endpoints is the endpoints of the relation, used to create a
	// relation key.
	Endpoints []domainrelation.Endpoint
}

// SubordinateUnitStatusHistoryData contains the data to start status
// history for both the unit and update the status history for the machine.
type SubordinateUnitStatusHistoryData struct {
	UnitName   string
	UnitStatus domainapplication.UnitStatusArg
}

// SubordinateCreated returns true if the data to set status history for a
// new subordinate unit is available, false otherwise.
func (s SubordinateUnitStatusHistoryData) SubordinateCreated() bool {
	return s.UnitName != ""
}

// SubordinateUnitCreationInfo contains the information required to make the
// storage arguments for a subordinate unit that would be created when a unit
// enters scope of a container scoped relation.
type SubordinateUnitCreationInfo struct {
	// SubordinateApplicationUUID is the UUID of the subordinate application
	// that the new subordinate unit belongs to.
	SubordinateApplicationUUID application.UUID

	// MachineNetNodeUUID is the net node UUID of the machine that hosts the
	// principal unit. Storage for the new subordinate unit is attached to
	// this net node.
	MachineNetNodeUUID domainnetwork.NetNodeUUID
}

// SubordinateUnitStorageArgs contains the storage arguments to use when
// creating a new IAAS subordinate unit.
type SubordinateUnitStorageArgs struct {
	// UnitStorageArgs describes the storage directives, instances and
	// attachments to create for the subordinate unit.
	UnitStorageArgs domainstorage.CreateUnitStorageArg

	// IAASUnitStorageArgs describes the machine ownership of the storage
	// entities created for the subordinate unit.
	IAASUnitStorageArgs domainstorage.CreateIAASUnitStorageArg
}
