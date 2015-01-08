// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import "github.com/juju/names"

// APICallerFunc is a function type that implements APICaller.
type APICallerFunc func(objType string, version int, id, request string, params, response interface{}) error

func (f APICallerFunc) APICall(objType string, version int, id, request string, params, response interface{}) error {
	return f(objType, version, id, request, params, response)
}

func (APICallerFunc) BestFacadeVersion(facade string) int {
	return 0
}

func (APICallerFunc) EnvironTag() (names.EnvironTag, error) {
	return names.NewEnvironTag(""), nil
}

func (APICallerFunc) Close() error {
	return nil
}
