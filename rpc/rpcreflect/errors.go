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
			return fmt.Sprintf("unknown version %d for facade type %q", e.Version, e.RootMethod)
		}
		return fmt.Sprintf("unknown facade type %q", e.RootMethod)
	}
	return fmt.Sprintf("unknown method %q at version %d for facade type %q", e.Method, e.Version, e.RootMethod)
}
