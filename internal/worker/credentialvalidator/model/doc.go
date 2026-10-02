// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package model provides the model-worker manifold variant of the
// credential validator: the credential-validity flag used by per-model
// worker engines on controllers, reading model credential status through
// local domain services instead of an API caller.
//
// The variant is a separate package because it depends on
// internal/services, whose transitive imports reach the Dqlite database
// layer and its CGO-bound SQLite driver. The parent package is part of
// the jujuagentd machine graph, which is built as a pure-Go binary with
// CGO disabled; sharing one package with this variant would pull the
// database closure into the machine agent's build. The dependency is
// one-way: this package imports the parent for the shared worker
// configuration, and the parent must never import this package or
// internal/services; the machine agent's CGO-free build fails if it
// does, by design.
package model
