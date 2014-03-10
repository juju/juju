// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"time"

	"github.com/juju/loggo"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/apiserver"
	coretesting "launchpad.net/juju-core/testing"
)

type stateSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&stateSuite{})

var testPingPeriod = 100 * time.Millisecond

func (s *stateSuite) TestConnectionBrokenDetection(c *gc.C) {
	s.PatchValue(&api.PingPeriod, testPingPeriod)

	st, _ := s.OpenAPIAsNewMachine(c)

	// Connection still alive
	select {
	case <-time.After(testPingPeriod):
	case <-st.Broken():
		c.Fatalf("connection should be alive still")
	}

	// Close the connection and see if we detect this
	go st.Close()

	// Check it's detected
	select {
	case <-time.After(testPingPeriod + time.Second):
		c.Fatalf("connection not closed as expected")
	case <-st.Broken():
		return
	}
}

func (s *stateSuite) TestPing(c *gc.C) {
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("ping-tester", tw, loggo.DEBUG), gc.IsNil)
	defer loggo.RemoveWriter("ping-tester")

	st, _ := s.OpenAPIAsNewMachine(c)
	err := st.Ping()
	c.Assert(err, gc.IsNil)
	err = st.Close()
	c.Assert(err, gc.IsNil)
	err = st.Ping()
	c.Assert(err, gc.Equals, rpc.ErrShutdown)

	// Make sure that ping messages have not been logged.
	for _, m := range tw.Log {
		c.Logf("checking %q", m.Message)
		c.Check(m.Message, gc.Not(gc.Matches), ".*Ping.*")
	}
}

func (s *stateSuite) TestClientNoNeedToPing(c *gc.C) {
	s.PatchValue(apiserver.MaxPingInterval, time.Duration(0))
	st, err := api.Open(s.APIInfo(c), api.DefaultDialOpts())
	c.Assert(err, gc.IsNil)
	time.Sleep(coretesting.ShortWait)
	err = st.Ping()
	c.Assert(err, gc.IsNil)
}

func (s *stateSuite) TestAgentConnectionShutsDownWithNoPing(c *gc.C) {
	s.PatchValue(apiserver.MaxPingInterval, time.Duration(0))
	st, _ := s.OpenAPIAsNewMachine(c)
	time.Sleep(coretesting.ShortWait)
	err := st.Ping()
	c.Assert(err, gc.ErrorMatches, "connection is shut down")
}
