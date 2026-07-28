// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package migrationreconciler provides a controller-scoped worker that
// completes interrupted target-side migration import aborts that their driver
// (the source controller's migrationmaster) did not finish, for example
// because the source controller went away or a process restarted.
//
// A v8 model import records a durable model_migration_import claim whose phase
// is the source of truth for the migration's fate. Once a claim leaves the
// importing phase it is committed to a terminal outcome:
//
//   - aborting: the controller-database import writes are undone and the model
//     database is staged for deletion, but the claim is deliberately left in
//     place until the drop is proven complete, then finalized (claim deleted).
//   - activating: the model has crossed the point of no return and may be live,
//     so it must never be torn down by this worker.
//
// This worker guarantees the aborting outcome holds even when the driver
// disappears: it drives aborting claims to their terminal (deleted) state. On
// the facade Abort path that finalization is synchronous; this worker is the
// crash-recovery fallback. Activating claims are not completed here - the
// worker only warns when one is stuck past a conservative age, since a model
// that may be live needs source/operator resolution, not target-side action.
//
// The worker follows the same pattern as the removal worker: a scan loop
// discovers claims and spawns a per-model abort worker via a worker.Runner for
// each aborting claim. Each abort worker re-drives its idempotent
// finalization for a single model; it is convergent, so the runner's
// restart-with-backoff eventually completes it (the abort worker simply exits
// on failure and is restarted after a delay, and exits cleanly once the claim
// is gone). Claims still in the importing phase cannot be completed by the
// target alone either; for those the worker likewise just warns when one is
// stuck, indicating a source controller that never finished or aborted the
// migration.
package migrationreconciler
