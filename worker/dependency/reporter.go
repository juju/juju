// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

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
