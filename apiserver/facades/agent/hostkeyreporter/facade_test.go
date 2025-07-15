// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	machine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type facadeSuite struct {
	testing.BaseSuite

	machineService *MockMachineService
}

func TestFacadeSuite(t *stdtesting.T) {
	tc.Run(t, &facadeSuite{})
}

func (s *facadeSuite) TestReportKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().GetMachineUUID(c.Context(), machine.Name("0")).Return(machine.UUID("0"), nil)
	s.machineService.EXPECT().SetSSHHostKeys(c.Context(), machine.UUID("0"), []string{"rsa0", "dsa0"}).Return(nil)

	facade, err := New(s.machineService, apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	})
	c.Assert(err, tc.ErrorIsNil)

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
	result, err := facade.ReportKeys(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: &params.Error{Message: "permission denied", Code: params.CodeUnauthorized}},
		},
	})
}

func (s *facadeSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.machineService = NewMockMachineService(ctrl)

	return ctrl
}
