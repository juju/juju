// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package annotation provides a way to attach key-value pairs to object types.
//
// The current implementation is limited to the following object types:
//
//   - Applications
//   - Charms
//   - Machines
//   - Models
//   - Storage
//   - Units
//
// The original intended purpose of annotations was to provide a way to
// attach metadata to objects in Juju for the GUI dashboard.
package annotation
