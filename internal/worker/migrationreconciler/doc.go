// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationreconciler

// Package migrationreconciler provides a controller-scoped worker that
// completes interrupted target-side migration import claims - both aborts and
// activations - that their driver (the source controller's migrationmaster) did
// not finish, for example because the source controller went away or a process
// restarted.
//
// A v8 model import records a durable model_migration_import claim whose phase
// is the source of truth for the migration's fate. Once a claim leaves the
// importing phase it is committed to a terminal outcome:
//
//   - aborting: the controller-database import writes are undone and the model
//     database is staged for deletion, but the claim is deliberately left in
//     place until the drop is proven complete, then finalized (claim deleted).
//   - activating: the model has crossed the point of no return and may be live,
//     so it must never be torn down; activation is instead driven to completion
//     via the idempotent finalization (clear the model gate, activate the model
//     row, delete the claim).
//
// This worker guarantees that guarantee holds even when the driver disappears:
// it drives both aborting and activating claims to their terminal (deleted)
// state. On the facade paths that finalization is synchronous; this worker is
// the crash-recovery fallback.
//
// The worker follows the same pattern as the removal worker: a scan loop
// discovers claims and, per phase, spawns a per-model job worker via a
// worker.Runner - an abort worker for aborting claims and an
// activation-completion worker for activating claims. Each job worker re-drives
// its idempotent finalization for a single model; both are convergent, so the
// runner's restart-with-backoff eventually completes them (the job worker simply
// exits on failure and is restarted after a delay, and exits cleanly once the
// claim is gone). Only claims still in the importing phase cannot be completed
// by the target alone; for those the worker just warns when one is stuck past a
// conservative age, indicating a source controller that never finished or
// aborted the migration.
