// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package life defines lifecycle state values for Juju entities.
// See the sections below for details about this package.
//
// # How this package works
//
// **Lifecycle states**: Juju entities (machines, units, applications, models)
// follow a lifecycle from Alive to Dying to Dead. Alive means the entity should
// exist and operate normally. Dying means the entity is scheduled for removal
// but may have cleanup dependencies. Dead means cleanup is complete and the
// entity can be removed from the database.
//
// **State transitions**: Valid transitions are Alive → Dying → Dead. Entities
// cannot transition backward (e.g., from Dead to Alive). The transition from
// Alive to Dying typically occurs when a user requests removal. The transition
// from Dying to Dead occurs when all cleanup tasks (removing relations,
// releasing resources, stopping units) are complete.
//
// **Predicates**: The package provides type-safe predicates (IsDead, IsAlive,
// IsNotAlive, IsNotDead) for checking lifecycle state. These predicates enable
// consistent state checks across the codebase.
//
// # How to use this package correctly
//
// **Validation**: Lifecycle values MUST be validated using Value.Validate()
// before persistence. Only the three defined constants (Alive, Dying, Dead) are
// valid.
package life
