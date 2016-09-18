// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/rpc"
	coretesting "github.com/juju/juju/testing"
)

type pingerSuite struct {
	apiserverBaseSuite
}

var _ = gc.Suite(&pingerSuite{})

var testPingPeriod = 100 * time.Millisecond

func (s *pingerSuite) newServerWithTestClock(c *gc.C) (*apiserver.Server, *testing.Clock) {
	clock := testing.NewClock(time.Now())
	config := s.sampleConfig(c)
	config.Clock = clock
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

	err := conn.Ping()
	c.Assert(err, jc.ErrorIsNil)
	err = conn.Close()
	c.Assert(err, jc.ErrorIsNil)
	err = conn.Ping()
	c.Assert(errors.Cause(err), gc.Equals, rpc.ErrShutdown)

	// Make sure that ping messages have not been logged.
	for _, m := range tw.Log() {
		c.Logf("checking %q", m.Message)
		c.Check(m.Message, gc.Not(gc.Matches), `.*"Request":"Ping".*`)
	}
}

func (s *pingerSuite) advanceClock(c *gc.C, clock *testing.Clock, delta time.Duration, count int) {
	for i := 0; i < count; i++ {
		clock.Advance(delta)
	}
}

func (s *pingerSuite) TestClientNoNeedToPing(c *gc.C) {
	server, clock := s.newServerWithTestClock(c)
	conn := s.OpenAPIAsAdmin(c, server)

	// Here we have a conundrum, we can't wait for a clock alarm because
	// one isn't set because we don't have pingers for clients. So just
	// a short wait then.
	time.Sleep(coretesting.ShortWait)

	s.advanceClock(c, clock, apiserver.MaxClientPingInterval, 2)
	err := conn.Ping()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *pingerSuite) checkConnectionDies(c *gc.C, conn api.Connection) {
	attempt := utils.AttemptStrategy{
		Total: coretesting.LongWait,
		Delay: coretesting.ShortWait,
	}
	for a := attempt.Start(); a.Next(); {
		err := conn.Ping()
		if err != nil {
			c.Assert(err, gc.ErrorMatches, "connection is shut down")
			return
		}
	}
	c.Fatal("connection didn't get shut down")
}

func (s *pingerSuite) TestAgentConnectionShutsDownWithNoPing(c *gc.C) {
	server, clock := s.newServerWithTestClock(c)
	conn, _ := s.OpenAPIAsNewMachine(c, server)

	s.advanceClock(c, clock, apiserver.MaxClientPingInterval, 2)
	s.checkConnectionDies(c, conn)
}

func (s *pingerSuite) TestAgentConnectionDelaysShutdownWithPing(c *gc.C) {
	server, clock := s.newServerWithTestClock(c)
	conn, _ := s.OpenAPIAsNewMachine(c, server)

	err := conn.Ping()
	c.Assert(err, jc.ErrorIsNil)

	attemptDelay := apiserver.MaxClientPingInterval / 2
	// As long as we don't wait too long, the connection stays open

	testStart := clock.Now()
	c.Logf(
		"pinging 10 times with %v delay, ping timeout %v, starting at %v",
		attemptDelay, apiserver.MaxClientPingInterval, testStart,
	)
	lastLoop := testStart
	for i := 0; i < 10; i++ {
		clock.Advance(attemptDelay)
		testNow := clock.Now()
		loopDelta := testNow.Sub(lastLoop)
		if lastLoop.IsZero() {
			loopDelta = 0
		}
		c.Logf("duration since last ping: %v", loopDelta)
		err = conn.Ping()
		if !c.Check(
			err, jc.ErrorIsNil,
			gc.Commentf(
				"ping timeout exceeded at %v (%v since the test start)",
				testNow, testNow.Sub(testStart),
			),
		) {
			c.Check(err, gc.ErrorMatches, "connection is shut down")
			return
		}
		lastLoop = clock.Now()
	}

	// However, once we stop pinging for too long, the connection dies
	s.advanceClock(c, clock, apiserver.MaxClientPingInterval, 2)
	s.checkConnectionDies(c, conn)
}

func (s *pingerSuite) TestAgentConnectionsShutDownWhenAPIServerDies(c *gc.C) {
	clock := testing.NewClock(time.Now())
	config := s.sampleConfig(c)
	config.Clock = clock
	server := s.newServerDirtyKill(c, config)
	conn, _ := s.OpenAPIAsNewMachine(c, server)

	err := conn.Ping()
	c.Assert(err, jc.ErrorIsNil)
	server.Kill()

	// We know this is less than the client ping interval.
	clock.Advance(apiserver.MongoPingInterval)
	s.checkConnectionDies(c, conn)
}
