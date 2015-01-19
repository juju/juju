// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"testing"

	gitjujutesting "github.com/juju/testing"
)

// MgoTestPackage should be called to register the tests for any package
// that requires a secure connection to a MongoDB server.
func MgoTestPackage(t *testing.T) {
	gitjujutesting.MgoTestPackage(t, Certs)
}
