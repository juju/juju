// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package operationpruner provides a worker that periodically prunes
// stored operation results and metadata according to model configuration.
//
// # Overview
//
// The pruner watches the model configuration for changes to the
// following settings and uses them to determine what data to prune:
//   - config.MaxActionResultsAge: maximum age to retain operation results.
//   - config.MaxActionResultsSize: maximum total size (in MB) of stored
//     operation results or logs
//
// On a fixed interval, configured via the worker Config.PruneInterval,
// the worker asks an OperationService to prune operations older than the
// configured age or when the total size exceeds the configured limit.
//
// Services
//
//   - ModelConfigService abstracts access to the model configuration and
//     provides a watcher used by the worker to stay up to date.
//   - OperationService performs the actual pruning when invoked by the
//     worker, given the current age and size limits.
//
// # Behavior
//
// When started, the worker:
//  1. Subscribes to model config changes.
//  2. Reads the initial MaxActionResultsAge and MaxActionResultsSizeMB.
//  3. On each tick of the prune interval, calls OperationService.PruneOperations
//     with the latest limits.
//  4. Updates limits whenever the relevant model config keys change.
//
// # Integration
//
// The worker is intended to be run by the Juju controller, for each model.
package operationpruner
