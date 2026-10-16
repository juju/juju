// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package dqliteconfigupdater provides a jujud worker that keeps this
// controller's local runtime.conf delivery cache in sync with the mutable
// Dqlite tuning values of the controller config.
//
// The controller config domain service is the authoritative mutable source
// for query-tracing-enabled, query-tracing-threshold and
// dqlite-busy-timeout. runtime.conf is only the durable per-controller
// delivery cache that the dbaccessor worker reads when it constructs the
// Dqlite application: the values are fixed for the life of that
// application, so a running Dqlite app cannot be reconfigured in place.
//
// The worker watches WatchControllerConfig. Events that carry none of the
// three keys cannot change the desired values and are ignored; on every
// other event the worker re-reads the mutable Dqlite tuning values and
// compares them with the local cache:
//
//   - matching values establish the baseline with no write and no
//     restart; the initial all-keys watcher event is a reconciliation
//     like any other, so a restart after a write does not loop;
//   - on a difference the worker writes all changed values together
//     with one atomic ChangeControllerRuntimeConfig write, and only
//     after that write succeeds does it return an error wrapping
//     worker.ErrRestartAgent, so the normal jujud/snap supervision path
//     restarts the controller and dbaccessor constructs Dqlite with the
//     new values;
//   - a controller config, runtime config read, validation or write
//     failure is returned without ErrRestartAgent: no restart is
//     requested, and the dependency engine recreates the worker with
//     its standard error-delay backoff policy.
//
// The worker never uses ControllerApplication.Restart, the runtime config
// voyeur, or the config-change socket for this change, and the Dqlite
// port is outside its scope: it is bootstrap-seeded local state. Only
// the singleton bootstrap controller is served; a sequenced
// multi-controller rollout is deferred to the controller HA work.
package dqliteconfigupdater
