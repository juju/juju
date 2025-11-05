// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package status provides domain types and services for managing entity status
// in Juju.
//
// Status represents the operational state of entities (machines, units,
// applications, models) and is a key part of Juju's user interface. The
// status domain provides consistent status representation and history tracking
// across all entity types.
//
// # Status Types
//
// Status includes:
//   - Status value: the current state (e.g., active, error, waiting)
//   - Status message: human-readable description
//   - Status data: additional structured information
//   - Status history: timestamped history of status changes
//
// # Entity Status
//
// Status is tracked for:
//   - Models: overall model health
//   - Applications: application state
//   - Units: unit operational status
//   - Machines: machine provisioning and operational state
//
// The status domain provides a consistent interface for recording and querying
// status across all entity types.
package status
