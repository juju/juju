// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"testing"

	"github.com/juju/juju/mongo"
	gitjujutesting "github.com/juju/testing"
)

// MgoTestPackage should be called to register the tests for any package
// that requires a secure connection to a MongoDB server.
func MgoTestPackage(t *testing.T) {
	gitjujutesting.MgoTestPackage(t, Certs)
}

// NewMongoInfo returns information suitable for
// connecting to the testing state server's mongo database.
func NewMongoInfo() *mongo.MongoInfo {
	return &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{gitjujutesting.MgoServer.Addr()},
			CACert: CACert,
		},
	}
}

// NewDialOpts returns configuration parameters for
// connecting to the testing state server.
func NewDialOpts() mongo.DialOpts {
	return mongo.DialOpts{
		Timeout: LongWait,
	}
}
