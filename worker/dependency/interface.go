// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker"
)

// Engine is a mechanism for persistently running named workers, managing the
// dependencies between them, and reporting on their status.
type Engine interface {

	// Engine's primary purpose is to implement Installer responsibilities.
	Installer

	// Engine also exposes human-comprehensible status data to its clients.
	Reporter

	// Engine is itself a Worker.
	worker.Worker
}

// Installer takes responsibility for persistently running named workers and
// managing the dependencies between them.
type Installer interface {

	// Install causes the implementor to accept responsibility for maintaining a
	// worker corresponding to the supplied manifold, restarting it when it
	// fails and when its inputs' workers change, until the Engine shuts down.
	Install(name string, manifold Manifold) error
}

// Manifold defines the behaviour of a node in an Engine's dependency graph. It's
// named for the "device that connects multiple inputs or outputs" sense of the
// word.
type Manifold struct {

	// Inputs lists the names of the manifolds which this manifold might use.
	// An engine will attempt to start a worker independent of the availability
	// of these inputs, and will restart the worker as the available inputs
	// change. If a worker has no dependencies, it should declare empty inputs.
	Inputs []string

	// Start is used to create a worker for the manifold. It must not be nil.
	// The supplied GetResourceFunc will return ErrMissing for any dependency
	// not named in Inputs, and will cease to function immediately after the
	// StartFunc returns: do not store references to it.
	//
	// Note that, while Start must exist, it doesn't *have* to *start* a worker
	// (although it must return either a worker or an error). That is to say: in
	// *some* circumstances, it's ok to wrap a worker under the management of a
	// separate component (e.g. the `worker/agent` Manifold itself) but this
	// approach should only be used:
	//
	//  * as a last resort; and
	//  * with clear justification.
	//
	// ...because it's a deliberate, and surprising, subversion of the dependency
	// model; and is thus much harder to reason about and implement correctly. In
	// particular, if you write a surprising start func, you can't safely declare
	// any inputs at all.
	Start StartFunc

	// Output is used to implement a GetResourceFunc for manifolds that declare
	// a dependency on this one; it can be nil if your manifold is a leaf node,
	// or if it exposes no services to its dependents.
	//
	// If you implement an Output func, be especially careful to expose sensible
	// types. Your `out` param should almost always be a pointer to an interface;
	// and, furthermore, to an interface that does *not* satisfy `worker.Worker`.
	//
	// (Consider the interface segregation principle: the *Engine* is reponsible
	// for the lifetimes of the backing workers, and for handling their errors.
	// Exposing those levers to your dependents as well can only encourage them
	// to use them, and vastly complicate the possible interactions.)
	//
	// And if your various possible clients might use different sets of features,
	// please keep those interfaces segregated as well: prefer to accept [a *Foo
	// or a *Bar] rather than just [a *FooBar] -- unless all your clients really
	// do want a FooBar resource.
	//
	// Even if the Engine itself didn't bother to track the types exposed per
	// dependency, it's still a useful prophylactic against complexity -- so
	// that when reading manifold code, it should be immediately clear both what
	// your dependencies *are* (by reading the names in the manifold config)
	// and what they *do* for you (by reading the start func and observing the
	// types in play).
	Output OutputFunc
}

// Manifolds conveniently represents several Manifolds.
type Manifolds map[string]Manifold

// StartFunc returns a worker or an error. All the worker's dependencies should
// be taken from the supplied GetResourceFunc; if no worker can be started due
// to unmet dependencies, it should return ErrMissing, in which case it will
// not be called again until its declared inputs change.
type StartFunc func(getResource GetResourceFunc) (worker.Worker, error)

// GetResourceFunc returns an indication of whether a named dependency can be
// satisfied. In particular:
//
//  * if the named resource does not exist, it returns ErrMissing
//  * if the named resource exists, and out is nil, it returns nil
//  * if the named resource exists, and out is non-nil, it returns the error
//    from the named resource manifold's output func (hopefully nil)
//
// Appropriate types for the out pointer depend upon the resource in question.
type GetResourceFunc func(name string, out interface{}) error

// ErrMissing can be returned by a StartFunc or a worker to indicate to
// the engine that it can't be usefully restarted until at least one of its
// dependencies changes. There's no way to specify *which* dependency you need,
// because that's a lot of implementation hassle for little practical gain.
var ErrMissing = errors.New("dependency not available")

// ErrBounce can be returned by a StartFunc or a worker to indicate to
// the engine that it should be restarted immediately, instead of
// waiting for ErrorDelay. This is useful for workers which restart
// themselves to alert dependents that an output has changed.
var ErrBounce = errors.New("restart immediately")

// ErrUninstall can be returned by a StartFunc or a worker to indicate to the
// engine that it can/should never run again, and that the originating manifold
// should be completely removed.
var ErrUninstall = errors.New("resource permanently unavailable")

// OutputFunc is a type coercion function for a worker generated by a StartFunc.
// When passed an out pointer to a type it recognises, it will assign a suitable
// value and return no error.
type OutputFunc func(in worker.Worker, out interface{}) error

// IsFatalFunc is used to configure an Engine such that, if any worker returns
// an error that satisfies the engine's IsFatalFunc, the engine will stop all
// its workers, shut itself down, and return the original fatal error via Wait().
type IsFatalFunc func(err error) bool

// WorstErrorFunc is used to rank fatal errors, to allow an Engine to return the
// single most important error it's encountered.
type WorstErrorFunc func(err0, err1 error) error

// Reporter defines an interface for extracting human-relevant information
// from a worker.
type Reporter interface {

	// Report returns a map describing the state of the receiver. It is expected
	// to be goroutine-safe.
	//
	// It is polite and helpful to use the Key* constants and conventions defined
	// and described in this package, where appropriate, but that's for the
	// convenience of the humans that read the reports; we don't and shouldn't
	// have any code that depends on particular Report formats.
	Report() map[string]interface{}
}

// The Key constants describe the constant features of an Engine's Report.
const (

	// KeyState applies to a worker; possible values are "starting", "started",
	// "stopping", or "stopped". Or it might be something else, in distant
	// Reporter implementations; don't make assumptions.
	KeyState = "state"

	// KeyError holds some relevant error. In the case of an Engine, this will be:
	//  * any internal error indicating incorrect operation; or
	//  * the most important fatal error encountered by any worker; or
	//  * nil, if none of the above apply;
	// ...and the value should not be presumed to be stable until the engine
	// state is "stopped".
	//
	// In the case of a manifold, it will always hold the most recent error
	// returned by the associated worker (or its start func); and will be
	// rewritten whenever a worker state is set to "started" or "stopped".
	//
	// In the case of a resource access, it holds any error encountered when
	// trying to find or convert the resource.
	KeyError = "error"

	// KeyManifolds holds a map of manifold name to further data (including
	// dependency inputs; current worker state; and any relevant report/error
	// for the associated current/recent worker.)
	KeyManifolds = "manifolds"

	// KeyReport holds an arbitrary map of information returned by a manifold
	// Worker that is also a Reporter.
	KeyReport = "report"

	// KeyInputs holds the names of the manifolds on which this one depends.
	KeyInputs = "inputs"

	// KeyResourceLog holds a slice representing the calls the current worker
	// made to its getResource func; the type of the output param; and any
	// error encountered.
	KeyResourceLog = "resource-log"

	// KeyName holds the name of some resource.
	KeyName = "name"

	// KeyType holds a string representation of the type by which a resource
	// was accessed.
	KeyType = "type"
)
