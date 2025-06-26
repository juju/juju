// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

// pingerSuite exercises the apiserver's ping timeout functionality
// from the outside. Ping API requests are made (or not) to a running
// API server to ensure that the server shuts down the API connection
// as expected once there's been no pings within the timeout period.
type pingerSuite struct {
	jujutesting.ApiServerSuite
}

func TestPingerSuite(t *testing.T) {
	tc.Run(t, &pingerSuite{})
}

func (s *pingerSuite) SetUpTest(c *tc.C) {
	s.Clock = testclock.NewDilatedWallClock(time.Millisecond)
	s.ApiServerSuite.SetUpTest(c)
}

func (s *pingerSuite) TestStub(c *tc.C) {
	c.Skipf(`This suite is missing tests for the following scenarios:

- Test connection broken detection using machines.
- Test ping
- Test agent connection shuts down with no ping.
- Test agent connection delays shutdown with ping.
- Test agent connection shut down when API server dies.`)
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

func pingConn(c *tc.C, conn api.Connection) error {
	version := conn.BestFacadeVersion("Pinger")
	return conn.APICall(c.Context(), "Pinger", version, "", "Ping", nil, nil)
}
