package api_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/api"
	coretesting "launchpad.net/juju-core/testing"
	"net"
	stdtesting "testing"
)

type suite struct {
	testing.JujuConnSuite
	State    *api.State
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
	srv := api.NewServer(s.JujuConnSuite.State)
	go func() {
		err := srv.Accept(l)
		if err != nil {
			log.Printf("api server exited with error: %v", err)
		}
	}()
	s.State, err = api.Open(&api.Info{
		Addr: l.Addr().String(),
	})
	c.Assert(err, IsNil)
}

func (s *suite) TearDownTest(c *C) {
	s.State.Close()
	s.listener.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

func (s *suite) TestAddMachine(c *C) {
	m0, err := s.State.AddMachine()
	c.Assert(err, ErrorMatches, "cannot add a new machine: new machine must be started with a machine worker")
	c.Assert(m0, IsNil)
	m0, err = s.State.AddMachine(api.MachinerWorker, api.MachinerWorker)
	c.Assert(err, ErrorMatches, "cannot add a new machine: duplicate worker: machiner")
	c.Assert(m0, IsNil)
	m0, err = s.State.AddMachine(api.MachinerWorker)
	c.Assert(err, IsNil)
	c.Assert(m0.Id, Equals, "0")
	m0, err = s.State.Machine("0")
	c.Assert(err, IsNil)
	c.Assert(m0.Id, Equals, "0")
	c.Assert(m0.Workers, DeepEquals, []api.WorkerKind{api.MachinerWorker})

	allWorkers := []api.WorkerKind{api.MachinerWorker, api.FirewallerWorker, api.ProvisionerWorker}
	m1, err := s.State.AddMachine(allWorkers...)
	c.Assert(err, IsNil)
	c.Assert(m1.Id, Equals, "1")
	c.Assert(m1.Workers, DeepEquals, allWorkers)

	m0, err = s.State.Machine("1")
	c.Assert(err, IsNil)
	c.Assert(m0.Id, Equals, "1")
	c.Assert(m0.Workers, DeepEquals, allWorkers)

	machines, err := s.State.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(machines, HasLen, 2)
	c.Assert(machines[0].Id, Equals, "0")
	c.Assert(machines[1].Id, Equals, "1")
}
