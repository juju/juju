// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

// Unit topics

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

// Metrics user topics

// AddMetricsUserTopic is used to request the creation of new user credentials
// to access the controller's metrics endpoint.
// The payload for an AddMetricsUserTopic is the UserInfo structure.
const AddMetricsUserTopic = "metrics.user.add"

// AddMetricsUserResponseTopic is the topic to respond to an add request.
// The payload is the UserResponse type below.
const AddMetricsUserResponseTopic = "metrics.user.add.response"

// RemoveMetricsUserTopic is used to request the removal of a user.
// The payload for a RemoveMetricsUserTopic is a string representing the username.
const RemoveMetricsUserTopic = "metrics.user.remove"

// RemoveMetricsUserResponseTopic is the topic to respond to a remove request.
// The payload is the UserResponse type below.
const RemoveMetricsUserResponseTopic = "metrics.user.remove.response"

// UserInfo contains the information (username and password) needed to create
// a new user.
type UserInfo struct {
	Username string
	Password string
}

// UserResponse is the error result from the user operation (nil if the
// operation was successful).
type UserResponse error
