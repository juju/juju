// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper_test

import (
	gc "launchpad.net/gocheck"

	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/peergrouper"
)

type InitiateSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&InitiateSuite{})

// TODO(natefinch) add a test that InitiateMongoServer works when
// we support upgrading of existing environments.

func (s *InitiateSuite) TestInitiateReplicaSet(c *gc.C) {
	var err error
	inst := &coretesting.MgoInstance{Params: []string{"--replSet", "juju"}}
	err = inst.Start(true)
	c.Assert(err, gc.IsNil)
	defer inst.Destroy()

	info := inst.DialInfo()
	args := peergrouper.InitiateMongoParams{
		DialInfo:       info,
		MemberHostPort: inst.Addr(),
	}

	err = peergrouper.MaybeInitiateMongoServer(args)
	c.Assert(err, gc.IsNil)

	// This would return a mgo.QueryError if a ReplicaSet
	// configuration already existed but we tried to create
	// one with replicaset.Initiate again.
	err = peergrouper.MaybeInitiateMongoServer(args)
	c.Assert(err, gc.IsNil)

	// TODO test login
}
