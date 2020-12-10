// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"testing"

	jujutesting "github.com/juju/testing"
)

// MgoTestPackage should be called to register the tests for any package
// that requires a connection to a MongoDB server.
//
// The server will be configured without SSL enabled, which slows down
// tests. For tests that care about security (which should be few), use
// MgoSSLTestPackage.
func MgoTestPackage(t *testing.T) {
	jujutesting.MgoServer.EnableReplicaSet = true
	jujutesting.MgoTestPackage(t, nil)
}

// MgoSSLTestPackage should be called to register the tests for any package
// that requires a secure (SSL) connection to a MongoDB server.
func MgoSSLTestPackage(t *testing.T) {
	jujutesting.MgoServer.EnableReplicaSet = true
	jujutesting.MgoTestPackage(t, Certs)
}
