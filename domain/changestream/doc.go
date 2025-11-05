// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package changestream provides the infrastructure for tracking and publishing
// entity changes within Juju.
//
// The changestream domain implements a change notification system that allows
// watchers to observe modifications to entities. This is used throughout Juju
// to propagate state changes efficiently without polling.
//
// # Key Concepts
//
// Change streams track:
//   - Entity creation, modification, and deletion events
//   - Change namespaces for different entity types
//   - Stream positions for resumable watching
//
// The change stream is the foundation for Juju's reactive architecture,
// enabling workers to respond immediately to state changes.
package changestream
