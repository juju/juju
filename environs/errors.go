// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

// NoEnvError indicates the default environment config file is missing.
type NoEnvError struct {
	error
}

// IsNoEnv returns if err is a NoEnvError.
func IsNoEnv(err error) bool {
	_, ok := err.(NoEnvError)
	return ok
}
