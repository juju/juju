// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state provides the persistence layer for cloud image metadata management,
// implementing database operations for cloud image metadata data.
//
// This package handles storage and retrieval of:
//   - Image metadata and versions
//   - Image availability by cloud/region
//   - Custom image metadata
package state
