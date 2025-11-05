// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state provides the persistence layer for the access domain,
// implementing database operations for user and permission management.
//
// This package handles storage and retrieval of:
//   - User accounts and authentication credentials
//   - Permission grants for models, controllers, and offers
//   - External user relationships and group semantics
package state
