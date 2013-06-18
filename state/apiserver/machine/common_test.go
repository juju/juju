package machine_test
import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	stdtesting "testing"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type commonSuite struct {
	testing.JujuConnSuite

	authorizer fakeAuthorizer

	machine0 *state.Machine
	machine1 *state.Machine
}

func (s *commonSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine so that we can login as its agent
	var err error
	s.machine0, err = s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	// Add another normal machine
	s.machine1, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	// Create a fakeAuthorizer so we can check permissions.
	s.authorizer = fakeAuthorizer{
		tag:          state.MachineTag(s.machine1.Id()),
		loggedIn:     true,
		manager:      false,
		machineAgent: true,
	}
}

// fakeAuthorizer implements the common.Authorizer interface.
type fakeAuthorizer struct {
	tag          string
	loggedIn     bool
	manager      bool
	machineAgent bool
}

func (fa fakeAuthorizer) IsLoggedIn() bool {
	return fa.loggedIn
}

func (fa fakeAuthorizer) AuthOwner(tag string) bool {
	return fa.tag == tag
}

func (fa fakeAuthorizer) AuthEnvironManager() bool {
	return fa.manager
}

func (fa fakeAuthorizer) AuthMachineAgent() bool {
	return fa.machineAgent
}
