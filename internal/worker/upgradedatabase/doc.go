// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package upgradedatabase provides a worker that coordinates DDL-based
// database schema upgrades when a Juju controller is upgraded to a new
// version.
//
// # Coordination model
//
// On a multi-controller setup, only one controller acts as the upgrade leader.
// The leader creates an upgrade record in the controller database, runs the
// upgrade, and unlocks the DBUpgradeCompleteLock on success. Follower
// controllers watch the upgrade state and unlock the lock when they see the
// upgrade complete. All controllers must register as "ready" before the
// upgrade can start.
//
// # Upgrade sequence
//
// The leader performs the upgrade in order:
//
//  1. Controller DDL: applies the controller database schema patch set.
//  2. Model DDLs: applies the model database schema patch set for each
//     model known to the controller.
//  3. Upgrade steps: runs version-windowed data migration functions that
//     cannot be expressed as declarative schema patches (e.g. cross-model
//     data transformations). These receive both the controller and model
//     database handles.
//
// The DDL steps are idempotent — on retry they detect already-applied patches
// and become no-ops. Upgrade steps must also be written to be idempotent to
// allow safe retry after partial failure.
//
// # Failure handling
//
//   - If any step fails, the upgrade is marked as failed. The next agent
//     restart creates a fresh upgrade and retries from the beginning
//     (since DDL steps are idempotent).
//   - The worker has a configurable timeout (default 20 minutes) to
//     prevent indefinite blocking.
//   - If marking the upgrade as failed itself fails, manual intervention
//     is required — the worker logs an error and exits.
//
// # Version windowing
//
// Upgrade steps are keyed by [VersionWindow], which defines a range of source
// versions. Only steps matching the controller's from-version are executed.
// This allows stepping across multiple patch versions in a single upgrade.
package upgradedatabase
