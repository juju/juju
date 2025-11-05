// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state provides the persistence layer for autocert cache,
// implementing database operations for TLS certificate storage.
//
// This package handles storage and retrieval of:
//   - Cached TLS certificates
//   - Certificate metadata and expiration
package state
