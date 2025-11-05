// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state provides the persistence layer for removal management,
// implementing database operations for removal data.
//
// This package handles storage and retrieval of:
//   - Entity lifecycle state (alive, dying, dead)
//   - Removal job scheduling
//   - Force removal flags
package state
