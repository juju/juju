// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package agent contains messages for all agents rather than controllers.
//
// All these structures are expected to be used with a SimpleHub that doesn't
// seralize, hence no serialization directives.
package agent

// StartUnitTopic is used to request one or more units to start.
// The payload for a StartUnitTopic is the Units structure.
const StartUnitTopic = "unit.start"

// StopUnitTopic is used to request one or more units to stop.
// The payload for a StopUnitTopic is the Units structure.
const StopUnitTopic = "unit.stop"

// UnitStatusTopic is used to request the current status for the units.
// There is no payload for this request.
const UnitStatusTopic = "unit.status"

// UnitStatusResponseTopic is the topic to respond to a status request.
// The payload is the Status type below.
const UnitStatusResponseTopic = "unit.status.response"

// Units provides a way to request start or stop multiple units.
type Units struct {
	Names []string
}

// Status is a map of unit name to the status value. An interace{} value is returned
// to allow for simple expansion later. The output of the status is expected to just
// show a nice string representation of the map.
type Status map[string]interface{}
