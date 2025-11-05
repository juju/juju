// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state provides the persistence layer for cloud management,
// implementing database operations for cloud definitions and regions.
//
// This package handles storage and retrieval of:
//   - Cloud definitions (type, endpoint, auth types)
//   - Cloud regions and their configurations
//   - Cloud capabilities and feature support
package state
