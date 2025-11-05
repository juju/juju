// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state provides the persistence layer for machine management,
// implementing database operations for machine data.
//
// This package handles storage and retrieval of:
//   - Machine definitions and lifecycle state
//   - Cloud instance information (instance ID, hardware characteristics)
//   - Network configuration and addresses
//   - Machine placement and container relationships
//   - Agent version and status information
package state
