// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

// StartUnitTopic is used to request one or more units to start.
// The payload for a StartUnitTopic is the Units structure.
const StartUnitTopic = "unit.start"

// StartUnitResponseTopic is the topic to respond to a start request.
// The payload is the StartStopResponse type below.
const StartUnitResponseTopic = "unit.start.response"

// StopUnitTopic is used to request one or more units to stop.
// The payload for a StopUnitTopic is the Units structure.
const StopUnitTopic = "unit.stop"

// StopUnitResponseTopic is the topic to respond to a stop request.
// The payload is the StartStopResponse type below.
const StopUnitResponseTopic = "unit.stop.response"

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

// StartStopResponse returns a map of the requested unit names, and
// whether they were stopped, started, or not found.
type StartStopResponse map[string]interface{}

// Status is a map of unit name to the status value. An interface{} value is returned
// to allow for simple expansion later. The output of the status is expected to just
// show a nice string representation of the map.
type Status map[string]interface{}
