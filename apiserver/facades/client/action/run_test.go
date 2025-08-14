// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/pkg/errors"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/client/action"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type runSuite struct {
	jujutesting.ApiServerSuite

	blockCommandService *action.MockBlockCommandService
	applicationService  *action.MockApplicationService
	modelInfoService    *action.MockModelInfoService

	client *action.ActionAPI
}

func TestRunSuite(t *stdtesting.T) {
	tc.Run(t, &runSuite{})
}

func (s *runSuite) TestBlockRunOnAllMachines(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// block all changes
	s.blockAllChanges(c, "TestBlockRunOnAllMachines")
	_, err := s.client.RunOnAllMachines(
		c.Context(),
		params.RunParams{
			Commands: "hostname",
			Timeout:  testing.LongWait,
		})
	s.assertBlocked(c, err, "TestBlockRunOnAllMachines")
}

func (s *runSuite) TestBlockRunMachineAndApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// block all changes
	s.blockAllChanges(c, "TestBlockRunMachineAndApplication")
	_, err := s.client.Run(
		c.Context(),
		params.RunParams{
			Commands:     "hostname",
			Timeout:      testing.LongWait,
			Machines:     []string{"0"},
			Applications: []string{"magic"},
		})
	s.assertBlocked(c, err, "TestBlockRunMachineAndApplication")
}

func (s *runSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
Running juju-exec with machine, application and unit targets.
Running juju-exec against all machines.
Running "Run" requires administrator privilege.
Running "RunOnAllMachines" requires administrator privilege.
`)
}

func (s *runSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.blockCommandService = action.NewMockBlockCommandService(ctrl)
	s.applicationService = action.NewMockApplicationService(ctrl)
	s.modelInfoService = action.NewMockModelInfoService(ctrl)

	var err error
	auth := apiservertesting.FakeAuthorizer{
		Tag: jujutesting.AdminUser,
	}
	modelUUID := coremodel.GenUUID(c)
	s.client, err = action.NewActionAPI(auth, action.FakeLeadership{}, s.applicationService, s.blockCommandService, s.modelInfoService, modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	return ctrl
}

func (s *runSuite) assertBlocked(c *tc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue, tc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), tc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *runSuite) blockAllChanges(c *tc.C, msg string) {
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return(msg, nil)
}
