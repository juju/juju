// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state provides the persistence layer for key updater operations,
// implementing database operations for retrieving authorized keys.
//
// This package handles retrieval of:
//   - Authorized keys from multiple sources
//   - Key aggregation for machines
package state
