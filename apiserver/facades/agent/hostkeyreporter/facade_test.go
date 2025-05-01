// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter_test

import (
	"context"

	"github.com/juju/names/v6"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/hostkeyreporter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
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

var _ = gc.Suite(&facadeSuite{})

func (s *facadeSuite) SetUpTest(c *gc.C) {
	s.backend = new(mockBackend)
	s.authorizer = new(apiservertesting.FakeAuthorizer)
	facade, err := hostkeyreporter.New(s.backend, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.facade = facade
}

func (s *facadeSuite) TestReportKeys(c *gc.C) {
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
	result, err := s.facade.ReportKeys(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: nil},
		},
	})
	s.backend.stub.CheckCalls(c, []jujutesting.StubCall{{
		FuncName: "SetSSHHostKeys",
		Args: []interface{}{
			names.NewMachineTag("1"),
			state.SSHHostKeys{"rsa1", "dsa1"},
		},
	}})
}

type mockBackend struct {
	stub jujutesting.Stub
}

func (backend *mockBackend) SetSSHHostKeys(tag names.MachineTag, keys state.SSHHostKeys) error {
	backend.stub.AddCall("SetSSHHostKeys", tag, keys)
	return nil
}
