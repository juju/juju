// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// WatcherRelationUnitsData contains data returned by the
// WatcherRelationUnitsData state method. This ensures that the
// order of the strings cannot be misinterpreted.
type WatcherRelationUnitsData struct {
	RelationEndpointUUID      string
	RelationUnitNS            string
	ApplicationSettingsHashNS string
	UnitSettingsHashNS        string
}
