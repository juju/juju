// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/peergrouper"
)

type InitiateSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&InitiateSuite{})

// TODO(natefinch) add a test that InitiateMongoServer works when
// we support upgrading of existing environments.

func (s *InitiateSuite) TestInitiateReplicaSet(c *gc.C) {
	var err error
	inst := &gitjujutesting.MgoInstance{Params: []string{"--replSet", "juju"}}
	err = inst.Start(coretesting.Certs)
	c.Assert(err, jc.ErrorIsNil)
	defer inst.Destroy()

	info := inst.DialInfo()
	args := peergrouper.InitiateMongoParams{
		DialInfo:       info,
		MemberHostPort: inst.Addr(),
	}

	err = peergrouper.MaybeInitiateMongoServer(args)
	c.Assert(err, jc.ErrorIsNil)

	// This would return a mgo.QueryError if a ReplicaSet
	// configuration already existed but we tried to create
	// one with replicaset.Initiate again.
	// ErrReplicaSetAlreadyInitiated is not a failure but an
	// indication that we tried to initiate an initiated rs
	err = peergrouper.MaybeInitiateMongoServer(args)
	c.Assert(err, gc.Equals, peergrouper.ErrReplicaSetAlreadyInitiated)

	// Make sure running InitiateMongoServer without forcing will behave
	// in the same way as MaybeInitiateMongoServer
	err = peergrouper.InitiateMongoServer(args, false)
	c.Assert(err, gc.Equals, peergrouper.ErrReplicaSetAlreadyInitiated)

	// Assert that passing Force to initiate will re-create the replicaset
	// even though it exists already
	err = peergrouper.InitiateMongoServer(args, true)
	c.Assert(err, jc.ErrorIsNil)

	// TODO test login
}
