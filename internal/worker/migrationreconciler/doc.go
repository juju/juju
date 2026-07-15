// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationreconciler

// Package migrationreconciler provides a controller-scoped worker that
// completes interrupted target-side migration import aborts. When a v8 model
// import is aborted, the durable model_migration_import claim is moved to the
// aborting phase, the controller-database import writes are undone, and the
// model database is staged for deletion, but the claim is deliberately left in
// place until the drop is proven complete. On the facade Abort path that
// finalization is synchronous; this worker is the crash-recovery fallback for
// aborts whose caller did not finish (a source controller that went away, a
// process restart).
//
// The worker follows the same pattern as the removal worker: a scan loop
// discovers aborting claims and spawns a per-model job worker for each one via
// a worker.Runner. Each job worker's sole responsibility is to finalize a
// single model's abort (re-driving compensation, waiting for the database
// drop, and releasing the claim). The runner handles restart-with-backoff
// automatically, so the job worker simply exits on failure and is restarted
// after a delay. The worker also warns about claims stuck in the importing or
// activating phase past a conservative age, which indicate a source controller
// that never completed or aborted a migration.
