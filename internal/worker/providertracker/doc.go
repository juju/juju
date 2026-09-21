// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package providertracker provides a worker that loads and manages a Juju
// model's cloud provider (environ for IAAS, broker for K8S) and keeps it
// updated in response to model configuration, cloud credential, and model
// lifecycle changes.
//
// # Architecture
//
// The package has two worker layers:
//
//   - Worker (providerworker.go): the top-level worker that wires domain
//     services together, manages provider lifecycle (including provider
//     teardown on model removal), and coordinates the inner tracker.
//
//   - trackerWorker (trackerworker.go): the inner worker that loads the
//     provider, exposes it to clients via Provider(), and watches for model
//     config changes (SetConfig), credential changes (SetCloudSpec, if the
//     provider supports CloudSpecSetter), and model lifecycle transitions
//     (stopping when the model is removed or dead).
//
// # Tracker types
//
// The worker can be configured as singular or multi via TrackerType:
//
//   - SingularType: only one tracker exists across the controller for the
//     given namespace. Used for controller-scoped providers.
//
//   - MultiType: each model gets its own tracker. Used for model-scoped
//     providers.
//
// # Ephemeral providers
//
// NewEphemeralProvider creates a one-shot provider from a fixed config and
// cloud spec. It does not watch for changes — if credentials change, the
// provider must be recreated. This is useful for short-lived operations
// such as bootstrapping.
package providertracker
