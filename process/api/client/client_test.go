package client_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	pc "github.com/juju/juju/process/api/client"
	"github.com/juju/juju/state"
)

type clientSuite struct {
	testing.JujuConnSuite

	st      *api.State
	machine *state.Machine
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.st, s.machine = s.OpenAPIAsNewMachine(c, state.JobManageEnviron)

	err := s.machine.SetProviderAddresses(network.NewAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestListEmptyProcesses(c *gc.C) {
	client := pc.NewProcessClient(s.st)

	processes, err := client.ListProcesses(s.machine.Tag().String())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(len(processes), gc.Equals, 0)
}
