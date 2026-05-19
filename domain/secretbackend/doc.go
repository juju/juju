package main
// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package secretbackend manages secret backend registration and selection.
// See the sections below for details about this package.
//
// See github.com/juju/juju/domain/secret for secret operations.
// See github.com/juju/juju/internal/secrets/provider for backend provider implementations.
//
// # How this package works
//
// **Secret backends**: Secret backends store sensitive data for secrets. Backend
// types include controller (built-in Juju backend), kubernetes (Kubernetes
// provider), and vault (Vault provider). Backends can be addressed by UUID or
// human-readable name.
//
// **Secret backend capabilities**: The secret backend domain defines and validates
// secret backend records (identity, type, config), creates, updates, lists, and
// deletes backends, tracks rotation metadata (token rotate interval and next
// rotation time) and emits changes consumable by watchers used by rotation
// workers, manages per-model backend selection (default model backend is
// automatically resolved to the most appropriate built-in backend based on model
// type - IAAS or CAAS), and restores the default backend selection when a
// user-defined backend is destroyed.
//
// **Secret backend identity**: A backend can be addressed by UUID or by
// human-readable name. Only one is required for lookups/updates.
//
// **Secret backend origin**: Backends created at bootstrap time are marked as
// origin 'built-in'. All other backends are marked as origin 'user'.
//
// **Secret backend configuration**: Each backend carries a Config map[string]any.
// Values are stored as JSON.
//
// **Secret backend rotation**: Service interface allows specifying a rotation
// interval for backends. When set, the next rotation time is computed, then
// recorded and updated upon rotation events. Rotation events are emitted by the
// state layer through a watcher.
//
// **Secret backend selection**: For a given model, the effective backend name can
// be retrieved and set through the service layer. The special provider value
// "auto" resolves to the built-in controller backend for IAAS models and to the
// built-in kubernetes backend for CAAS models.
//
// **Built-in Kubernetes secret backend**: For CAAS models, a built-in secret
// backend is available, represented by a single entry in the secret_backend table
// named 'kubernetes'. Although multiple CAAS models might use this backend, there
// is only one database entry. When retrieving the backend configuration for a
// specific CAAS model, Juju dynamically constructs the configuration using the
// model's cloud and credential specifications (for endpoint information) and the
// model's name (which maps to the Kubernetes namespace). All CAAS models share
// the same backend type and database record, but their effective run-time
// configuration is model-specific.
//
// # How to use this package correctly
//
// **Immutability**: Built-in backends MUST NOT be modified or deleted. Backend
// types MUST NOT change after creation.
//
// **Validation**: Backend configuration keys MUST be non-empty strings. Values
// MUST be non-nil. String values MUST be non-empty.
package secretbackend
