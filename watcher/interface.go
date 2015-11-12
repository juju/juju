// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"github.com/juju/juju/worker"
)

// CoreWatcher encodes some features of a watcher. The most obvious one:
//
//     Changes() <-chan <T>
//
// ...can't be expressed cleanly; and this is annoying because every such chan
// needs to share common behaviours for the abstraction to be generally helpful.
// The critical features of a Changes chan are as follows:
//
//    * The channel should never be closed.
//    * The channel should send a single baseline value, representing the change
//      from a nil state; and subsequently send values representing deltas from
//      whatever had previously been sent.
//    * The channel should really never be closed. Many existing watchers *do*
//      close their channels when the watcher stops; this is harmful because it
//      mixes lifetime-handling into change-handling at the cost of clarity (and
//      in some cases correctness). So long as a watcher implements Worker, it
//      can be safely managed with the worker/catacomb package; client code ofc
//      still needs to check for closed channels (never trust a contract...) but
//      can treat that scenario as a simple error.
//
// To convert a state/watcher.Watcher to a CoreWatcher, replace Stop() and Err()
// with the boilerplate Kill() and Wait() implementations; and ensure that the
// watcher no longer closes its Changes channel.
type CoreWatcher interface {
	worker.Worker
}
