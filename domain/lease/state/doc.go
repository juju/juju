// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state provides the persistence layer for lease management,
// implementing database operations for lease data.
//
// This package handles storage and retrieval of:
//   - Lease definitions and holders
//   - Lease expiration times
//   - Lease claims and extensions
package state
