// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/juju/core/life"
	domainapplication "github.com/juju/juju/domain/application"
	domainrelation "github.com/juju/juju/domain/relation"
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
