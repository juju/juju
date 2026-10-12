// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state loads archived database dumps into the freshly
// bootstrapped controller and model databases during recovery bootstrap.
//
// The load is a one-shot operation run before any worker starts: one
// transaction per database, foreign key checks deferred while replaying,
// foreign_key_check proving integrity before commit. Every archived
// table replaces the target's rows except the few tables selected by service policy
// that carry the replacement's identity or runtime state. The
// replay restores the source controller identity and rebuilds object store
// placements for the single replacement node.
package state
