// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/pkg/errors"
	gomock "go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/action"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
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

var _ = tc.Suite(&runSuite{})

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
`)
}

func (s *runSuite) TestRunRequiresAdmin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	alpha := names.NewUserTag("alpha@bravo")
	modelUUID := modeltesting.GenModelUUID(c)
	auth := apiservertesting.FakeAuthorizer{
		Tag:         alpha,
		HasWriteTag: alpha,
	}
	st := s.ControllerModel(c).State()
	client, err := action.NewActionAPI(st, nil, auth, action.FakeLeadership{}, s.applicationService, s.blockCommandService, s.modelInfoService, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.Run(c.Context(), params.RunParams{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)

	auth.AdminTag = alpha
	client, err = action.NewActionAPI(st, nil, auth, action.FakeLeadership{}, s.applicationService, s.blockCommandService, s.modelInfoService, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.Run(c.Context(), params.RunParams{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *runSuite) TestRunOnAllMachinesRequiresAdmin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(model.ModelInfo{Type: model.IAAS}, nil)

	alpha := names.NewUserTag("alpha@bravo")
	auth := apiservertesting.FakeAuthorizer{
		Tag:         alpha,
		HasWriteTag: alpha,
	}
	st := s.ControllerModel(c).State()
	client, err := action.NewActionAPI(st, nil, auth, action.FakeLeadership{}, s.applicationService, s.blockCommandService, s.modelInfoService, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.RunOnAllMachines(c.Context(), params.RunParams{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)

	auth.AdminTag = alpha
	client, err = action.NewActionAPI(st, nil, auth, action.FakeLeadership{}, s.applicationService, s.blockCommandService, s.modelInfoService, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.RunOnAllMachines(c.Context(), params.RunParams{})
	c.Assert(err, tc.ErrorIsNil)
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
	modelUUID := modeltesting.GenModelUUID(c)
	s.client, err = action.NewActionAPI(s.ControllerModel(c).State(), nil, auth, action.FakeLeadership{}, s.applicationService, s.blockCommandService, s.modelInfoService, modelUUID)
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
