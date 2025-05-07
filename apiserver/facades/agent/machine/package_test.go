// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package machine_test -destination package_mock_test.go github.com/juju/juju/apiserver/facades/agent/machine NetworkService,MachineService
//go:generate go run go.uber.org/mock/mockgen -typed -package machine_test -destination facade_mock_test.go github.com/juju/juju/apiserver/facade WatcherRegistry

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type commonSuite struct {
	testing.ApiServerSuite

	authorizer apiservertesting.FakeAuthorizer

	machine0 *state.Machine
	machine1 *state.Machine
}

func (s *commonSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)

	st := s.ControllerModel(c).State()

	var err error
	s.machine0, err = st.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)

	s.machine1, err = st.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 1 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machine1.Tag(),
	}
}
