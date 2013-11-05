// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"time"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/state/api"
)

type stateSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&stateSuite{})

var testPingPeriod = 100 * time.Millisecond

func (s *stateSuite) TestConnectionBrokenDetection(c *gc.C) {
	origPingPeriod := api.PingPeriod
	api.PingPeriod = testPingPeriod
	defer func() {
		api.PingPeriod = origPingPeriod
	}()

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
