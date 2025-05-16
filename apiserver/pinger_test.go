// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
)

// pingerSuite exercises the apiserver's ping timeout functionality
// from the outside. Ping API requests are made (or not) to a running
// API server to ensure that the server shuts down the API connection
// as expected once there's been no pings within the timeout period.
type pingerSuite struct {
	jujutesting.ApiServerSuite
}

func TestPingerSuite(t *stdtesting.T) { tc.Run(t, &pingerSuite{}) }
func (s *pingerSuite) SetUpTest(c *tc.C) {
	s.Clock = testclock.NewDilatedWallClock(time.Millisecond)
	s.ApiServerSuite.SetUpTest(c)
}

func (s *pingerSuite) TestConnectionBrokenDetection(c *tc.C) {
	conn, _ := s.OpenAPIAsNewMachine(c)

	s.Clock.Advance(api.PingPeriod)
	// Connection still alive
	select {
	case <-conn.Broken():
		c.Fatalf("connection should be alive still")
	case <-time.After(coretesting.ShortWait):
		// all good, connection still there
	}

	conn.Close()

	s.Clock.Advance(api.PingPeriod + time.Second)
	// Check it's detected
	select {
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("connection not closed as expected")
	case <-conn.Broken():
		return
	}
}

func (s *pingerSuite) TestPing(c *tc.C) {
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("ping-tester", tw), tc.IsNil)

	conn, _ := s.OpenAPIAsNewMachine(c)

	c.Assert(pingConn(c, conn), tc.ErrorIsNil)
	c.Assert(conn.Close(), tc.ErrorIsNil)
	c.Assert(errors.Cause(pingConn(c, conn)), tc.Equals, rpc.ErrShutdown)

	// Make sure that ping messages have not been logged.
	for _, m := range tw.Log() {
		c.Logf("checking %q", m.Message)
		c.Check(m.Message, tc.Not(tc.Matches), `.*"Request":"Ping".*`)
	}
}

func (s *pingerSuite) TestClientNoNeedToPing(c *tc.C) {
	conn := s.OpenControllerModelAPI(c)

	// Here we have a conundrum, we can't wait for a clock alarm because
	// one isn't set because we don't have pingers for clients. So just
	// a short wait then.
	time.Sleep(coretesting.ShortWait)

	s.Clock.Advance(apiserver.MaxClientPingInterval * 2)
	time.Sleep(coretesting.ShortWait)
	c.Assert(pingConn(c, conn), tc.ErrorIsNil)
}

func (s *pingerSuite) TestAgentConnectionShutsDownWithNoPing(c *tc.C) {
	coretesting.SkipFlaky(c, "lp:1627086")
	conn, _ := s.OpenAPIAsNewMachine(c)

	s.Clock.Advance(apiserver.MaxClientPingInterval * 2)
	checkConnectionDies(c, conn)
}

func (s *pingerSuite) TestAgentConnectionDelaysShutdownWithPing(c *tc.C) {
	coretesting.SkipFlaky(c, "lp:1632485")
	conn, _ := s.OpenAPIAsNewMachine(c)

	// As long as we don't wait too long, the connection stays open
	attemptDelay := apiserver.MaxClientPingInterval / 2
	for i := 0; i < 10; i++ {
		s.Clock.Advance(attemptDelay)
		c.Assert(pingConn(c, conn), tc.ErrorIsNil)
	}

	// However, once we stop pinging for too long, the connection dies
	s.Clock.Advance(apiserver.MaxClientPingInterval * 2)
	checkConnectionDies(c, conn)
}

func (s *pingerSuite) TestAgentConnectionsShutDownWhenAPIServerDies(c *tc.C) {
	conn, _ := s.OpenAPIAsNewMachine(c)

	err := pingConn(c, conn)
	c.Assert(err, tc.ErrorIsNil)
	s.Server.Kill()

	checkConnectionDies(c, conn)
}

func checkConnectionDies(c *tc.C, conn api.Connection) {
	select {
	case <-conn.Broken():
	case <-time.After(coretesting.LongWait):
		c.Fatal("connection didn't get shut down")
	}
}

func pingConn(c *tc.C, conn api.Connection) error {
	version := conn.BestFacadeVersion("Pinger")
	return conn.APICall(c.Context(), "Pinger", version, "", "Ping", nil, nil)
}
