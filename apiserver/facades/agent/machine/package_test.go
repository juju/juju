// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"os"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package machine_test -destination package_mock_test.go github.com/juju/juju/apiserver/facades/agent/machine NetworkService,MachineService,ApplicationService,StatusService,RemovalService
//go:generate go run go.uber.org/mock/mockgen -typed -package machine_test -destination facade_mock_test.go github.com/juju/juju/apiserver/facade WatcherRegistry

func TestMain(m *stdtesting.M) {
	os.Exit(func() int {
		defer coretesting.MgoTestMain()()
		return m.Run()
	}())
}

type commonSuite struct {
	testing.ApiServerSuite

	authorizer apiservertesting.FakeAuthorizer
}

func (s *commonSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 1 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("1"),
	}
}
