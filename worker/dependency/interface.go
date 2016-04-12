// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker"
)

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

	// Filter is used to convert errors returned from workers or Start funcs. It
	// can be nil, in which case no filtering or conversion will be done.
	//
	// It's intended to convert domain-specific errors into dependency-specific
	// errors (such as ErrBounce and ErrUninstall), so that workers managed by
	// an Engine don't have to depend on this package directly.
	//
	// It *could* also be used to cause side effects, but remember to be careful;
	// from your perspective, it'll be called from an arbitrary goroutine.
	Filter FilterFunc

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

// Context represents the situation in which a StartFunc is running. A Context should
// not be used outside its StartFunc; attempts to do so will have undefined results.
type Context interface {

	// Abort will be closed if the containing engine no longer wants to
	// start the manifold's worker. You can ignore Abort if your worker
	// will start quickly -- it'll just be shut down immediately, nbd --
	// but if you need to mess with channels or long-running operations
	// in your StartFunc, Abort lets you do so safely.
	Abort() <-chan struct{}

	// Get returns an indication of whether a named dependency can be
	// satisfied. In particular:
	//
	//  * if the named resource does not exist, it returns ErrMissing;
	//  * else if out is nil, it returns nil;
	//  * else if the named resource has no OutputFunc, it returns ErrMissing;
	//  * else it passes out into the OutputFunc and returns whatever error
	//    transpires (hopefully nil).
	//
	// Appropriate types for the out pointer depend upon the resource in question.
	Get(name string, out interface{}) error
}

// StartFunc returns a worker or an error. All the worker's dependencies should
// be taken from the supplied GetResourceFunc; if no worker can be started due
// to unmet dependencies, it should return ErrMissing, in which case it will
// not be called again until its declared inputs change.
type StartFunc func(context Context) (worker.Worker, error)

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

// FilterFunc is an error conversion function for errors returned from workers
// or StartFuncs.
type FilterFunc func(error) error

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
