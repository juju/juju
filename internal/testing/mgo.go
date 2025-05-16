// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	stdtesting "testing"
	"time"

	mgotesting "github.com/juju/mgo/v3/testing"
)

// MgoTestPackage should be called to register the tests for any package
// that requires a connection to a MongoDB server.
//
// The server will be configured without SSL enabled, which slows down
// tests. For tests that care about security (which should be few), use
// MgoSSLTestPackage.
func MgoTestPackage(t *stdtesting.T) {
	mgotesting.MgoServer.EnableReplicaSet = true
	// Tests tend to cause enough contention that the default lock request
	// timeout of 5ms is not enough. We may need to consider increasing the
	// value for production also.
	mgotesting.MgoServer.MaxTransactionLockRequestTimeout = 20 * time.Millisecond
	mgotesting.MgoTestPackage(t, nil)
}

// MgoSSLTestPackage should be called to register the tests for any package
// that requires a secure (SSL) connection to a MongoDB server.
func MgoSSLTestPackage(t *stdtesting.T) {
	mgotesting.MgoServer.EnableReplicaSet = true
	// Tests tend to cause enough contention that the default lock request
	// timeout of 5ms is not enough. We may need to consider increasing the
	// value for production also.
	mgotesting.MgoServer.MaxTransactionLockRequestTimeout = 20 * time.Millisecond
	mgotesting.MgoTestPackage(t, Certs)
}
