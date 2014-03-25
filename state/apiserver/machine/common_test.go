package machine_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	coretesting "launchpad.net/juju-core/testing"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type commonSuite struct {
	testing.JujuConnSuite

	authorizer apiservertesting.FakeAuthorizer

	machine0 *state.Machine
	machine1 *state.Machine
}

func (s *commonSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	s.machine0, err = s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)

	s.machine1, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 1 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:          s.machine1.Tag(),
		LoggedIn:     true,
		MachineAgent: true,
	}
}
