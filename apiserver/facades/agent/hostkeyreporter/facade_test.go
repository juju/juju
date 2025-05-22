// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facades/agent/hostkeyreporter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type facadeSuite struct {
	testing.BaseSuite
	backend    *mockBackend
	authorizer *apiservertesting.FakeAuthorizer
	facade     *hostkeyreporter.Facade
}

func TestFacadeSuite(t *stdtesting.T) {
	tc.Run(t, &facadeSuite{})
}

func (s *facadeSuite) SetUpTest(c *tc.C) {
	s.backend = new(mockBackend)
	s.authorizer = new(apiservertesting.FakeAuthorizer)
	facade, err := hostkeyreporter.New(s.backend, s.authorizer)
	c.Assert(err, tc.ErrorIsNil)
	s.facade = facade
}

func (s *facadeSuite) TestReportKeys(c *tc.C) {
	s.authorizer.Tag = names.NewMachineTag("1")

	args := params.SSHHostKeySet{
		EntityKeys: []params.SSHHostKeys{
			{
				Tag:        names.NewMachineTag("0").String(),
				PublicKeys: []string{"rsa0", "dsa0"},
			}, {
				Tag:        names.NewMachineTag("1").String(),
				PublicKeys: []string{"rsa1", "dsa1"},
			},
		},
	}
	result, err := s.facade.ReportKeys(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: nil},
		},
	})
	s.backend.stub.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "SetSSHHostKeys",
		Args: []interface{}{
			names.NewMachineTag("1"),
			state.SSHHostKeys{"rsa1", "dsa1"},
		},
	}})
}

type mockBackend struct {
	stub testhelpers.Stub
}

func (backend *mockBackend) SetSSHHostKeys(tag names.MachineTag, keys state.SSHHostKeys) error {
	backend.stub.AddCall("SetSSHHostKeys", tag, keys)
	return nil
}
