// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state provides the persistence layer for change stream management,
// implementing database operations for tracking entity changes.
//
// This package handles storage and retrieval of:
//   - Change event records
//   - Entity change metadata
//   - Change stream cursors
package state
