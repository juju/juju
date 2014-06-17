// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

// Caller is implemented by the client-facing State object.
type Caller interface {
	// Call makes a call to the API server with the given object type,
	// id, request and parameters. The response is filled in with the
	// call's result if the call is successful.
	Call(objType string, version int, id, request string, params, response interface{}) error

	// BestFacadeVersion returns the newest version of 'objType' that this
	// client can use with the current API server.
	BestFacadeVersion(facade string) int
}

// FacadeCaller is a wrapper around the common paradigm that a given client
// just wants to make calls on a facade using the best known version of the API.
type FacadeCaller interface {
	// APICall will place a request against the API using the requested
	// Facade and the best version that the API server supports that is
	// also known to the client.
	APICall(request string, params, response interface{}) error

	// BestAPIVersion returns the API version that we were able to
	// determine is supported by both the client and the API Server
	BestAPIVersion() int

	// RawCaller returns the wrapped Caller. This can be used if you need
	// to switch what Facade you are calling (such as Facades that return
	// Watchers and then need to use the Watcher facade)
	RawCaller() Caller
}

// BestAPIVersioner is a type that exports a BestAPIVersion() function.
// The client side of Facades should export BestAPIVersion so that code using
// them is able to determine if they need to use a compatibility code path.
type BestAPIVersioner interface {
	BestAPIVersion() int
}

type facadeCaller struct {
	facade string
	caller Caller
}

func (fc facadeCaller) APICall(request string, params, response interface{}) error {
	return fc.caller.Call(
		fc.facade, fc.caller.BestFacadeVersion(fc.facade), "",
		request, params, response)
}

func (fc facadeCaller) BestAPIVersion() int {
	return fc.caller.BestFacadeVersion(fc.facade)
}

func (fc facadeCaller) RawCaller() Caller {
	return fc.caller
}

// GetFacadeCaller wraps a Caller for a given Facade
func GetFacadeCaller(caller Caller, facade string) FacadeCaller {
	return facadeCaller{facade: facade, caller: caller}
}
