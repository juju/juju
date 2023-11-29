// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bootstrap ensures that when the initial bootstrap process
// has started that the agent binary and the controller application along with
// the charm is correctly seeded into the object store.
//
// The intention is that the worker is started as soon as possible during
// bootstrap and that it will uninstall itself once the bootstrap process
// has completed.
//
// Underlying all these changes is to ensure that we do not expose the
// database outside of the dependency engine. This should prevent any side
// loading of the database, which makes it easier to reason about and prevents
// flaky tests (see machine legacy tests as an example).
package bootstrap
