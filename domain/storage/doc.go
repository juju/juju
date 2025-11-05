// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package storage provides the domain for managing storage pools and
// configuration in Juju.
//
// Storage in Juju allows applications to request persistent storage volumes
// for their units. The storage domain manages storage pool definitions,
// provider configurations, and storage constraints.
//
// # Key Concepts
//
// Storage pools define:
//   - Provider type (e.g., ebs, ceph, lvm)
//   - Pool-specific configuration attributes
//   - Storage capabilities and constraints
//
// Storage can be:
//   - Filesystem storage: mounted as a directory
//   - Block storage: raw block devices
//
// # Storage Lifecycle
//
// Storage pools are:
//   - Defined during bootstrap or created later
//   - Referenced by storage constraints in applications
//   - Used by the storage provisioner to create volumes
//   - Managed separately from the applications using them
package storage
