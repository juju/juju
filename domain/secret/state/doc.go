// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state provides the persistence layer for secret management,
// implementing database operations for secret data.
//
// This package handles storage and retrieval of:
//   - Secret metadata and ownership
//   - Secret revisions and content references
//   - Access grants and permissions
//   - Rotation policies and schedules
package state
