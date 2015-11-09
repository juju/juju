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
// For this reason, you must *not* convert a pre-existing watcher by just adding
// Kill and Wait methods; you must also stop the implementation from closing its
// changes chan.
//
// In addition to the above, all currently extant watchers implement, and are
// accessed in terms of, Stop() and Err() methods, even though they're not very
// helpful. See worker/catacomb/doc.go for a discussion of the problems with
// those methods; for now, just hold your nose and implement them, but don't
// use them.
type CoreWatcher interface {
	worker.Worker

	// Stop calls Kill() and returns Wait(). It's a bad method and it should
	// feel bad, and so should you if you use it in new code.
	Stop() error

	// Err returns the internal tomb's Err(). It's a bad method and it should
	// feel bad, and so should you if you use it in new code.
	Err() error
}

// NotifyChan is a change channel as described in the CoreWatcher documentation.
// It sends a single value to indicate that the watch is active, and subsequent
// values whenever the value under observation changes.
type NotifyChan <-chan struct{}

// A NotifyWatcher conveniently ties a NotifyChan to the worker.Worker that
// represents its validity.
type NotifyWatcher interface {
	CoreWatcher
	Changes() NotifyChan
}

// StringsChan is a change channel as described in the CoreWatcher documentation.
// It sends a single value indicating a baseline set of values, and subsequent
// values representing additions, changes, and removals of those values. The
// precise semantics depend upon the individual watcher.
type StringsChan <-chan []string

// A StringsWatcher conveniently ties a StringsChan to the worker.Worker that
// represents its validity.
type StringsWatcher interface {
	CoreWatcher
	Changes() StringsChan
}
