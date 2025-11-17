// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package secretbackend defines the secret backend domain.
//
// # What does this domain do?
//
// The secret backend domain:
//   - defines and validates secret backend records (identity, type, config).
//   - creates, updates, lists, and deletes backends, with safeguards for
//     immutability and in-use protection in the state layer.
//   - tracks rotation metadata (token rotate interval and next rotation time)
//     and emitting changes consumable by watchers used by rotation workers.
//   - manages per-model backend selection. The default model backend is
//     automatically resolved to the most appropriate built-in backend based on
//     model type (IAAS or CAAS).
//   - restores the default backend selection when a user-defined backend is
//     destroyed, respecting the model type.
//
// # How does this domain work?
//
//   - Backend identity: A backend can be addressed by UUID or by human-readable
//     name. Only one is required for lookups/updates.
//   - Backend type: Backend types are either `controller` for the built-in backend
//     into Juju controller, `kubernetes` for the Kubernetes provider, or `vault` for
//     the Vault provider. Types are represented by string constants.
//   - Backend origin: Backends created at bootstrap time are marked as origin
//     'built-in'. All other backends are marked as origin 'user'. 'built-in'
//     backends are immutable and cannot be deleted.
//   - Backend configuration: Each backend carries a Config `map[string]any`.
//     Keys must be non-empty. Values must be non-nil and, when strings,
//     non-empty. Values are stored as JSON.
//   - Backend auth token rotation: Service interface allows specifying a rotation interval for
//     backends. When set, the next rotation time is computed, then recorded and
//     updated upon rotation events. Rotation events are emitted by
//     the state layer through a watcher.
//   - Default backend selection: For a given model, the effective backend name
//     can be retrieved and set through the service layer. The special provider
//     value “auto” resolves to the built-in `controller` backend for IAAS
//     models and to the built-in `kubernetes` backend for CAAS models.
package secretbackend
