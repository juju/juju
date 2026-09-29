// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package watcher defines the watcher types of the Juju core, and the workers
// that react to them.
//
// A Watcher[T] is a worker that reports changes of type T on the channel
// returned by its Changes method; the channel is closed when the watcher is
// killed. The package defines the common shapes:
//
//   - NotifyWatcher sends a single value to indicate that the watch is
//     active, and subsequent values whenever the value or values under
//     observation change.
//   - StringsWatcher sends a slice of strings indicating a baseline set of
//     values, and subsequent values representing changes of values.
//
// The remaining files define the watchers of specific domains, such as
// secrets, relations, offers and migrations.
//
// NewNotifyWorker and NewStringsWorker start a worker that runs a handler in
// response to a NotifyWatcher or a StringsWatcher respectively. A handler
// (NotifyHandler or StringsHandler) provides SetUp, which returns the watcher,
// Handle, which is called for each value the watcher sends, and TearDown. If
// SetUp or Handle returns an error the worker stops. Each worker runs in a
// catacomb.
//
// See github.com/juju/worker/v5 for the worker.Worker interface and the
// catacomb.
package watcher
