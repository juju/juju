// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpcreflect

import (
	"fmt"
)

// CallNotImplementedError is the error returned when an attempt to call to
// an unknown API method is made.
type CallNotImplementedError struct {
	RootMethod string
	Version    int
	Method     string
}

// Error implements the error interface.
func (e *CallNotImplementedError) Error() string {
	if e.Method == "" {
		if e.Version != 0 {
			return fmt.Sprintf("unknown version (%d) of interface %q", e.Version, e.RootMethod)
		}
		return fmt.Sprintf("unknown object type %q", e.RootMethod)
	}
	methodVersion := e.RootMethod
	if e.Version != 0 {
		methodVersion = fmt.Sprintf("%s(%d)", e.RootMethod, e.Version)
	}
	return fmt.Sprintf("no such request - method %s.%s is not implemented", methodVersion, e.Method)
}
