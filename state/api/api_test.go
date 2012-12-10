package api_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	coretesting "launchpad.net/juju-core/testing"
	"net"
	"runtime"
	"sync"
	stdtesting "testing"
	"time"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

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
	var err error
	s.srv, err = api.NewServer(s.State, "localhost:0", []byte(coretesting.ServerCert), []byte(coretesting.ServerKey))
	c.Assert(err, IsNil)
	s.APIState, err = api.Open(&api.Info{
		Addr:   s.srv.Addr(),
		CACert: []byte(coretesting.CACert),
	})
	c.Assert(err, IsNil)
}

func (s *suite) TearDownTest(c *C) {
	err := s.srv.Stop()
	c.Assert(err, IsNil)
	s.JujuConnSuite.TearDownTest(c)
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

func (s *suite) TestConcurrentRequests(c *C) {
	c.Skip("concurrent requests not implemented yet")
	ms := make([]*state.Machine, 3)
	for i := range ms {
		var err error
		ms[i], err = s.State.AddMachine(state.MachinerWorker)
		c.Assert(err, IsNil)
		err = ms[i].SetInstanceId(state.InstanceId(fmt.Sprintf("m-%d", i)))
		c.Assert(err, IsNil)
	}
	var wg sync.WaitGroup
	wg.Add(len(ms))
	for i, m := range ms {
		i := i
		m := m
		go func() {
			expectId := fmt.Sprintf("m-%d", i)
			for j := 0; ; j++ {
				c.Logf("request %d.%d", i, j)
				id, err := s.APIState.Request(m.Id())
				if !c.Check(err, IsNil) {
					break
				}
				c.Check(id, Equals, expectId)
				runtime.Gosched()
			}
			wg.Done()
		}()
	}
	time.Sleep(100 * time.Millisecond)
	err := s.srv.Stop()
	c.Assert(err, IsNil)
	wg.Wait()
}
