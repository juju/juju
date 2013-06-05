// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

// Caller interface is implemented by the client-facing State object.
type Caller interface {
	Call(objType, id, request string, params, response interface{}) error
}
