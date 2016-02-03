// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import "github.com/juju/juju/api/base"

// FacadeCallerFunc is a function type that implements FacadeCaller.
// The only method that actually does anything is FacadeCall itself
// which calls the function. The other methods are just stubs.
type FacadeCallerFunc func(request string, params, response interface{}) error

// FacadeCall will place a request against the API using the requested
// Facade and the best version that the API server supports that is
// also known to the client.
func (f FacadeCallerFunc) FacadeCall(request string, params, response interface{}) error {
	return f(request, params, response)
}

func (FacadeCallerFunc) Name() string {
	return "fake"
}

func (FacadeCallerFunc) BestAPIVersion() int {
	return 0
}

func (FacadeCallerFunc) RawAPICaller() base.APICaller {
	return APICallerFunc(func(objType string, version int, id, request string, params, response interface{}) error {
		return nil
	})
}
