// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"time"
)

type stateSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&stateSuite{})

var testPingPeriod = 100 * time.Millisecond

func (s *stateSuite) TestConnectionBrokenDetection(c *C) {
	stm, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	err = stm.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, IsNil)
	err = stm.SetPassword("password")
	c.Assert(err, IsNil)

	origPingPeriod := api.PingPeriod
	api.PingPeriod = testPingPeriod
	defer func() {
		api.PingPeriod = origPingPeriod
	}()

	st := s.OpenAPIAsMachine(c, stm.Tag(), "password", "fake_nonce")
	defer st.Close()

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
