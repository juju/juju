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
	l, err := net.Listen("tcp", ":0")
	c.Assert(err, IsNil)
	s.listener = l
	go api.Serve(s.State, l, []byte(coretesting.ServerCert), []byte(coretesting.ServerKey))
	s.APIState, err = api.Open(&api.Info{
		Addr:   l.Addr().String(),
		CACert: []byte(coretesting.CACert),
	})
	c.Assert(err, IsNil)
}

func (s *suite) TearDownTest(c *C) {
	s.listener.Close()
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
