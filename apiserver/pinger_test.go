// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/rpc"
	coretesting "github.com/juju/juju/testing"
)

// pingerSuite exercises the apiserver's ping timeout functionality
// from the outside. Ping API requests are made (or not) to a running
// API server to ensure that the server shuts down the API connection
// as expected once there's been no pings within the timeout period.
type pingerSuite struct {
	apiserverBaseSuite
}

var _ = gc.Suite(&pingerSuite{})

func (s *pingerSuite) newServerWithTestClock(c *gc.C) (*apiserver.Server, *testclock.Clock) {
	clock := testclock.NewClock(time.Now())
	config := s.config
	config.PingClock = clock
	server := s.newServer(c, config)
	return server, clock
}

func (s *pingerSuite) TestConnectionBrokenDetection(c *gc.C) {
	server, clock := s.newServerWithTestClock(c)
	conn, _ := s.OpenAPIAsNewMachine(c, server)

	clock.Advance(api.PingPeriod)
	// Connection still alive
	select {
	case <-conn.Broken():
		c.Fatalf("connection should be alive still")
	case <-time.After(coretesting.ShortWait):
		// all good, connection still there
	}

	conn.Close()

	clock.Advance(api.PingPeriod + time.Second)
	// Check it's detected
	select {
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("connection not closed as expected")
	case <-conn.Broken():
		return
	}
}

func (s *pingerSuite) TestPing(c *gc.C) {
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("ping-tester", tw), gc.IsNil)

	server, _ := s.newServerWithTestClock(c)
	conn, _ := s.OpenAPIAsNewMachine(c, server)

	c.Assert(pingConn(conn), jc.ErrorIsNil)
	c.Assert(conn.Close(), jc.ErrorIsNil)
	c.Assert(errors.Cause(pingConn(conn)), gc.Equals, rpc.ErrShutdown)

	// Make sure that ping messages have not been logged.
	for _, m := range tw.Log() {
		c.Logf("checking %q", m.Message)
		c.Check(m.Message, gc.Not(gc.Matches), `.*"Request":"Ping".*`)
	}
}

func (s *pingerSuite) TestClientNoNeedToPing(c *gc.C) {
	server, clock := s.newServerWithTestClock(c)
	conn := s.OpenAPIAsAdmin(c, server)

	// Here we have a conundrum, we can't wait for a clock alarm because
	// one isn't set because we don't have pingers for clients. So just
	// a short wait then.
	time.Sleep(coretesting.ShortWait)

	clock.Advance(apiserver.MaxClientPingInterval * 2)
	time.Sleep(coretesting.ShortWait)
	c.Assert(pingConn(conn), jc.ErrorIsNil)
}

func (s *pingerSuite) TestAgentConnectionShutsDownWithNoPing(c *gc.C) {
	coretesting.SkipFlaky(c, "lp:1627086")
	server, clock := s.newServerWithTestClock(c)
	conn, _ := s.OpenAPIAsNewMachine(c, server)

	waitAndAdvance(c, clock, apiserver.MaxClientPingInterval*2)
	checkConnectionDies(c, conn)
}

func (s *pingerSuite) TestAgentConnectionDelaysShutdownWithPing(c *gc.C) {
	coretesting.SkipFlaky(c, "lp:1632485")
	server, clock := s.newServerWithTestClock(c)
	conn, _ := s.OpenAPIAsNewMachine(c, server)

	// As long as we don't wait too long, the connection stays open
	attemptDelay := apiserver.MaxClientPingInterval / 2
	for i := 0; i < 10; i++ {
		waitAndAdvance(c, clock, attemptDelay)
		c.Assert(pingConn(conn), jc.ErrorIsNil)
	}

	// However, once we stop pinging for too long, the connection dies
	waitAndAdvance(c, clock, apiserver.MaxClientPingInterval*2)
	checkConnectionDies(c, conn)
}

func (s *pingerSuite) TestAgentConnectionsShutDownWhenAPIServerDies(c *gc.C) {
	server := s.newServerDirtyKill(c, s.config)
	conn, _ := s.OpenAPIAsNewMachine(c, server)

	err := pingConn(conn)
	c.Assert(err, jc.ErrorIsNil)
	server.Kill()

	checkConnectionDies(c, conn)
}

func waitAndAdvance(c *gc.C, clock *testclock.Clock, delta time.Duration) {
	waitForClock(c, clock)
	clock.Advance(delta)
}

func waitForClock(c *gc.C, clock *testclock.Clock) {
	select {
	case <-clock.Alarms():
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for clock")
	}
}

func checkConnectionDies(c *gc.C, conn api.Connection) {
	select {
	case <-conn.Broken():
	case <-time.After(coretesting.LongWait):
		c.Fatal("connection didn't get shut down")
	}
}

func pingConn(conn api.Connection) error {
	version := conn.BestFacadeVersion("Pinger")
	return conn.APICall("Pinger", version, "", "Ping", nil, nil)
}
