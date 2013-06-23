// +build ignore

// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"time"
)

type stateSuite struct {
	baseSuite
}

var _ = Suite(&stateSuite{})

var testPingPeriod = 100 * time.Millisecond

func (s *stateSuite) TestConnectionBrokenDetection(c *C) {
	stm, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	origPingPeriod := api.PingPeriod
	api.PingPeriod = testPingPeriod
	defer func() {
		api.PingPeriod = origPingPeriod
	}()

	st := s.openAs(c, stm.Tag())
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
