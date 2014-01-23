// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

// Caller is implemented by the client-facing State object.
type Caller interface {
	// Call makes a call to the API server with the given object type,
	// id, request and parameters. The response is filled in with the
	// call's result if the call is successful.
	Call(objType, id, request string, params, response interface{}) error
}
