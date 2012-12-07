package api_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	coretesting "launchpad.net/juju-core/testing"
	"net"
	stdtesting "testing"
)

type suite struct {
	testing.JujuConnSuite
	APIState *api.State
	listener net.Listener
	srv      *api.Server
}

var _ = Suite(&suite{})

func (s *suite) SetUpSuite(c *C) {
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *suite) TearDownSuite(c *C) {
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *suite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.srv = api.NewServer(s.State, "localhost:16463", []byte(coretesting.ServerCert), []byte(coretesting.ServerKey))
	var err error
	s.APIState, err = api.Open(&api.Info{
		Addr:   "localhost:16463",
		CACert: []byte(coretesting.CACert),
	})
	c.Assert(err, IsNil)
}

func (s *suite) TearDownTest(c *C) {
	err := s.srv.Stop()
	c.Assert(err, IsNil)
	s.JujuConnSuite.TearDownTest(c)
}

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

func (s *suite) TestRequest(c *C) {
	m, err := s.State.AddMachine(state.MachinerWorker)
	c.Assert(err, IsNil)
	instId, err := s.APIState.Request(m.Id())
	c.Check(instId, Equals, "")
	c.Assert(err, ErrorMatches, "instance id for machine 0 not found")

	err = m.SetInstanceId("foo")
	c.Assert(err, IsNil)

	instId, err = s.APIState.Request(m.Id())
	c.Assert(err, IsNil)
	c.Assert(instId, Equals, "foo")
}
