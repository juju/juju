// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package operation defines the domain model, contracts, and documentation for
// Juju operations.
//
// An operation represents a higher-level unit of work initiated by a user.
// There is two type of operations:
//   - action operations are defined by the charm and implemented as scripts in
//     the charm
//   - exec operations are shell commands.
//
// An operation is divided as several tasks, each of which is executed in a specific
// unit or machine.
//
// The operation domain provides the types and services to:
//   - execute operations on specific targets (applications, machines or units)
//   - query the status of operations in batch or through filters
//   - manage operations (cancel, prune)
//
// The operation domain is consumed by client facades and worker such as:
//   - apiserver/facades/client/action to list, query, and manage operations.
//   - internal/worker/uniter to execute tasks on units.
//   - internal/worker/machineactions to execute tasks on machines.
package operation
