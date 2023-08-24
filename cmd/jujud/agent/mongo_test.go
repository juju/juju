// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	mgotesting "github.com/juju/mgo/v3/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/mongo"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/peergrouper"
)

type mongoSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&mongoSuite{})

func (s *mongoSuite) TestStateWorkerDialSetsWriteMajority(c *gc.C) {
	s.testStateWorkerDialSetsWriteMajority(c, true)
}

func (s *mongoSuite) TestStateWorkerDialDoesNotSetWriteMajorityWithoutReplsetConfig(c *gc.C) {
	s.testStateWorkerDialSetsWriteMajority(c, false)
}

func (s *mongoSuite) testStateWorkerDialSetsWriteMajority(c *gc.C, configureReplset bool) {
	inst := mgotesting.MgoInstance{
		EnableReplicaSet: true,
	}
	err := inst.Start(coretesting.Certs)
	c.Assert(err, jc.ErrorIsNil)
	defer inst.Destroy()

	dialOpts := stateWorkerDialOpts
	dialOpts.Timeout = coretesting.LongWait
	if configureReplset {
		info := inst.DialInfo()
		info.Timeout = dialOpts.Timeout
		args := peergrouper.InitiateMongoParams{
			DialInfo:       info,
			MemberHostPort: inst.Addr(),
		}
		err = peergrouper.InitiateMongoServer(args)
		c.Assert(err, jc.ErrorIsNil)
	} else {
		dialOpts.Direct = true
	}

	mongoInfo := mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{inst.Addr()},
			CACert: coretesting.CACert,
		},
	}
	session, err := mongo.DialWithInfo(mongoInfo, dialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	safe := session.Safe()
	c.Assert(safe, gc.NotNil)
	c.Assert(safe.WMode, gc.Equals, "majority")
	c.Assert(safe.J, jc.IsTrue) // always enabled
}
