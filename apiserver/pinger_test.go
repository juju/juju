// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"time"

	"github.com/juju/loggo"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	coretesting "github.com/juju/juju/testing"
)

type pingerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&pingerSuite{})

var testPingPeriod = 100 * time.Millisecond

func (s *pingerSuite) TestConnectionBrokenDetection(c *gc.C) {
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

func (s *pingerSuite) TestPing(c *gc.C) {
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("ping-tester", tw, loggo.DEBUG), gc.IsNil)
	defer loggo.RemoveWriter("ping-tester")

	st, _ := s.OpenAPIAsNewMachine(c)
	err := st.Ping()
	c.Assert(err, jc.ErrorIsNil)
	err = st.Close()
	c.Assert(err, jc.ErrorIsNil)
	err = st.Ping()
	c.Assert(err, gc.Equals, rpc.ErrShutdown)

	// Make sure that ping messages have not been logged.
	for _, m := range tw.Log() {
		c.Logf("checking %q", m.Message)
		c.Check(m.Message, gc.Not(gc.Matches), `.*"Request":"Ping".*`)
	}
}

func (s *pingerSuite) TestClientNoNeedToPing(c *gc.C) {
	s.PatchValue(apiserver.MaxClientPingInterval, time.Duration(0))
	st, err := api.Open(s.APIInfo(c), api.DefaultDialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	time.Sleep(coretesting.ShortWait)
	err = st.Ping()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *pingerSuite) TestAgentConnectionShutsDownWithNoPing(c *gc.C) {
	s.PatchValue(apiserver.MaxClientPingInterval, time.Duration(0))
	st, _ := s.OpenAPIAsNewMachine(c)
	time.Sleep(coretesting.ShortWait)
	err := st.Ping()
	c.Assert(err, gc.ErrorMatches, "connection is shut down")
}

func (s *pingerSuite) TestAgentConnectionDelaysShutdownWithPing(c *gc.C) {
	// We have to be careful, because Login can take 25ms, so we ping
	// immediately after connecting.
	s.PatchValue(apiserver.MaxClientPingInterval, 50*time.Millisecond)
	st, _ := s.OpenAPIAsNewMachine(c)
	err := st.Ping()
	c.Assert(err, jc.ErrorIsNil)
	// As long as we don't wait too long, the connection stays open
	for i := 0; i < 10; i++ {
		time.Sleep(10 * time.Millisecond)
		err = st.Ping()
		c.Assert(err, jc.ErrorIsNil)
	}
	// However, once we stop pinging for too long, the connection dies
	time.Sleep(75 * time.Millisecond)
	err = st.Ping()
	c.Assert(err, gc.ErrorMatches, "connection is shut down")
}

type mongoPingerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&mongoPingerSuite{})

func (s *mongoPingerSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	// We need to set the ping interval before the server is started in test setup.
	restore := gitjujutesting.PatchValue(apiserver.MongoPingInterval, coretesting.ShortWait)
	s.AddSuiteCleanup(func(*gc.C) { restore() })
}

func (s *mongoPingerSuite) TestAgentConnectionsShutDownWhenStateDies(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c)
	err := st.Ping()
	c.Assert(err, jc.ErrorIsNil)
	gitjujutesting.MgoServer.Destroy()

	attempt := utils.AttemptStrategy{
		Total: coretesting.LongWait,
		Delay: coretesting.ShortWait,
	}
	for a := attempt.Start(); a.Next(); {
		if err := st.Ping(); err != nil {
			c.Assert(err, gc.ErrorMatches, "connection is shut down")
			return
		}
	}
	c.Fatalf("timed out waiting for API server to die")
}
