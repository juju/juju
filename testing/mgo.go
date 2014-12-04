// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"testing"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/mongo"
)

// MgoTestPackage should be called to register the tests for any package
// that requires a secure connection to a MongoDB server.
func MgoTestPackage(t *testing.T) {
	gitjujutesting.MgoTestPackage(t, Certs)
}

// NewServerReplSet returns a new mongo server instance to use in
// testing with replicaset. The caller is responsible for calling
// inst.Destroy() when done.
func NewServerReplSet(c *gc.C) *gitjujutesting.MgoInstance {
	inst := &gitjujutesting.MgoInstance{Params: []string{"--replSet", mongo.ReplicaSetName}}
	err := inst.Start(Certs)
	c.Assert(err, jc.ErrorIsNil)

	// Verify the server is up before returning.

	session, err := inst.DialDirect()
	if err != nil {
		inst.Destroy()
		c.Fatalf("error dialing mongo server: %v", err.Error())
	}
	defer session.Close()

	session.SetMode(mgo.Monotonic, true)
	if err = session.Ping(); err != nil {
		inst.Destroy()
		c.Fatalf("error pinging mongo server: %v", err.Error())
	}
	return inst
}
