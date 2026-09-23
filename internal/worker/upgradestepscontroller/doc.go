// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package upgradestepscontroller provides a worker that runs controller-level
// upgrade steps after the database schema upgrade has completed.
//
// # Position in the upgrade pipeline
//
// The overall Juju upgrade sequence is:
//
//  1. upgradedatabase: applies DDL schema patches to controller and model
//     databases, leaving the upgrade in DBCompleted state.
//  2. upgradestepscontroller (this worker): runs code-level upgrade steps
//     (PreUpgradeSteps, PerformUpgradeSteps) for the Controller and
//     HostMachine targets. Each controller runs its own steps and marks
//     itself done. All controllers wait for each other.
//  3. The last controller to finish archives the upgrade.
//
// # Coordination
//
// On a multi-controller setup, every controller runs this worker. Each
// controller:
//
//   - Subscribes to state watchers for StepsCompleted and Error.
//   - Runs its upgrade steps in a sub-worker (controllerStepsWorker).
//   - On success: calls SetControllerDone, then waits for all other
//     controllers to complete (StepsCompleted event).
//   - On failure: calls SetDBUpgradeFailed and exits.
//
// The UpgradeCompleteLock is unlocked only when all controllers have
// completed, allowing downstream workers (which depend on the lock) to
// start.
//
// # Failure modes
//
//   - State not DBCompleted: aborts — the upgradedatabase worker must
//     have failed. Manual intervention required.
//   - Upgrade step failure: aborts with SetDBUpgradeFailed.
//   - API connection lost mid-upgrade: returns the error to trigger an
//     agent restart (the upgrade is not marked as failed).
//   - Timeout: the worker has a configurable timeout for waiting on
//     other controllers.
package upgradestepscontroller
