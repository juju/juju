// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"github.com/juju/names"
)

// APICaller is implemented by the client-facing State object.
type APICaller interface {
	// APICall makes a call to the API server with the given object type,
	// id, request and parameters. The response is filled in with the
	// call's result if the call is successful.
	APICall(objType string, version int, id, request string, params, response interface{}) error

	// BestFacadeVersion returns the newest version of 'objType' that this
	// client can use with the current API server.
	BestFacadeVersion(facade string) int

	// EnvironTag returns the tag of the environment the client is
	// connected to.
	EnvironTag() (names.EnvironTag, error)
}

// FacadeCaller is a wrapper for the common paradigm that a given client just
// wants to make calls on a facade using the best known version of the API. And
// without dealing with an id parameter.
type FacadeCaller interface {
	// FacadeCall will place a request against the API using the requested
	// Facade and the best version that the API server supports that is
	// also known to the client.
	FacadeCall(request string, params, response interface{}) error

	// Name returns the facade name.
	Name() string

	// BestAPIVersion returns the API version that we were able to
	// determine is supported by both the client and the API Server
	BestAPIVersion() int

	// RawAPICaller returns the wrapped APICaller. This can be used if you need
	// to switch what Facade you are calling (such as Facades that return
	// Watchers and then need to use the Watcher facade)
	RawAPICaller() APICaller
}

type facadeCaller struct {
	facadeName  string
	bestVersion int
	caller      APICaller
}

var _ FacadeCaller = facadeCaller{}

// FacadeCall will place a request against the API using the requested
// Facade and the best version that the API server supports that is
// also known to the client. (id is always passed as the empty string.)
func (fc facadeCaller) FacadeCall(request string, params, response interface{}) error {
	return fc.caller.APICall(
		fc.facadeName, fc.bestVersion, "",
		request, params, response)
}

// Name returns the facade name.
func (fc facadeCaller) Name() string {
	return fc.facadeName
}

// BestAPIVersion returns the version of the Facade that is going to be used
// for calls. It is determined using the algorithm defined in api
// BestFacadeVersion. Callers can use this to determine what methods must be
// used for compatibility.
func (fc facadeCaller) BestAPIVersion() int {
	return fc.bestVersion
}

// RawAPICaller returns the wrapped APICaller. This can be used if you need to
// switch what Facade you are calling (such as Facades that return Watchers and
// then need to use the Watcher facade)
func (fc facadeCaller) RawAPICaller() APICaller {
	return fc.caller
}

// NewFacadeCaller wraps an APICaller for a given facade name and the
// best available version.
func NewFacadeCaller(caller APICaller, facadeName string) FacadeCaller {
	return NewFacadeCallerForVersion(caller, facadeName, caller.BestFacadeVersion(facadeName))
}

// NewFacadeCallerForVersion wraps an APICaller for a given facade
// name and version.
func NewFacadeCallerForVersion(caller APICaller, facadeName string, version int) FacadeCaller {
	return facadeCaller{
		facadeName:  facadeName,
		bestVersion: version,
		caller:      caller,
	}
}
