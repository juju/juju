// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	stdtesting "testing"

	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/mongo"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/peergrouper"
)

type mongoSuite struct {
	coretesting.BaseSuite
}

func TestMongoSuite(t *stdtesting.T) { tc.Run(t, &mongoSuite{}) }
func (s *mongoSuite) TestStateWorkerDialSetsWriteMajority(c *tc.C) {
	s.testStateWorkerDialSetsWriteMajority(c, true)
}

func (s *mongoSuite) TestStateWorkerDialDoesNotSetWriteMajorityWithoutReplsetConfig(c *tc.C) {
	s.testStateWorkerDialSetsWriteMajority(c, false)
}

func (s *mongoSuite) testStateWorkerDialSetsWriteMajority(c *tc.C, configureReplset bool) {
	inst := mgotesting.MgoInstance{
		EnableReplicaSet: true,
	}
	err := inst.Start(coretesting.Certs)
	c.Assert(err, tc.ErrorIsNil)
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
		c.Assert(err, tc.ErrorIsNil)
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
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	safe := session.Safe()
	c.Assert(safe, tc.NotNil)
	c.Assert(safe.WMode, tc.Equals, "majority")
	c.Assert(safe.J, tc.IsTrue) // always enabled
}
