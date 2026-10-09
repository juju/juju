// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package recovery defines the domain vocabulary for controller recovery
// from a backup archive: the validated archive summary (ArchiveInfo,
// ModelInfo, ApplicationInfo) and the preflight gates over it
// (CheckAgentVersion, ModelFamily, CheckProviderFamily). The replacement
// controller is bootstrapped with the source's identity and its
// databases loaded from the archived dumps; the summary and gates are
// what the bootstrap client decides from.
//
// The offline archive handling — reading the tarball, verifying its
// checksum, and decoding the dumps into the summary — lives in
// internal/recovery, which composes this domain.
//
// The state subpackage owns the agent-side database load. It runs inside
// dqlite bootstrap, after the schema migrations and before any agent, API
// or worker starts, with every database exclusively owned by bootstrap.
// That exclusivity is why the load takes plain *sql.DB handles rather
// than a transaction-runner factory: it drives one connection with
// foreign keys disabled, a single transaction per database and a
// foreign_key_check before commit — mechanics a pooled runner cannot
// express.
package recovery
