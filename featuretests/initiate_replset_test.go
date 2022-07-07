// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/peergrouper"
)

type InitiateSuite struct {
	coretesting.BaseSuite
}

func (s *InitiateSuite) TestInitiateReplicaSet(c *gc.C) {
	var err error
	inst := &jujutesting.MgoInstance{EnableReplicaSet: true}
	err = inst.Start(coretesting.Certs)
	c.Assert(err, jc.ErrorIsNil)
	defer inst.Destroy()

	info := inst.DialInfo()
	args := peergrouper.InitiateMongoParams{
		DialInfo:       info,
		MemberHostPort: inst.Addr(),
	}

	err = peergrouper.InitiateMongoServer(args)
	c.Assert(err, jc.ErrorIsNil)

	// Calling initiate again will re-create the replicaset even though it exists already
	err = peergrouper.InitiateMongoServer(args)
	c.Assert(err, jc.ErrorIsNil)

	// TODO test login
}
