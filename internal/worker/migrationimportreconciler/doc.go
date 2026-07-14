// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package migrationimportreconciler provides a controller-scoped worker that
// completes interrupted target-side migration import aborts. When a v8 model
// import is aborted, the durable model_migration_import claim is moved to the
// aborting phase, the controller-database import writes are undone, and the
// model database is staged for deletion, but the claim is deliberately left in
// place until the drop is proven complete. On the facade Abort path that
// finalization is synchronous; this worker is the crash-recovery fallback for
// aborts whose caller did not finish (a source controller that went away, a
// process restart). It periodically scans for aborting claims, re-drives the
// abort compensation (idempotent), and finalizes the abort by deleting the
// claim once the undertaker's model-database deleter has dropped the database.
// It also warns about claims stuck in the importing or activating phase past a
// conservative age, which indicate a source controller that never completed or
// aborted a migration.
package migrationimportreconciler
