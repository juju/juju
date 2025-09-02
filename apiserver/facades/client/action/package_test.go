// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/leadership"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/action ApplicationService,ModelInfoService,OperationService
//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination leader_mock_test.go github.com/juju/juju/core/leadership Reader
//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination blockservices_mock_test.go github.com/juju/juju/apiserver/common BlockCommandService

type MockBaseSuite struct {
	Authorizer          *facademocks.MockAuthorizer
	Leadership          *MockReader
	BlockCommandService *MockBlockCommandService
	ApplicationService  *MockApplicationService
	ModelInfoService    *MockModelInfoService
	OperationService    *MockOperationService
}

func (s *MockBaseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.BlockCommandService = NewMockBlockCommandService(ctrl)
	s.ApplicationService = NewMockApplicationService(ctrl)
	s.ModelInfoService = NewMockModelInfoService(ctrl)
	s.OperationService = NewMockOperationService(ctrl)
	s.Leadership = NewMockReader(ctrl)
	s.Authorizer = facademocks.NewMockAuthorizer(ctrl)

	return ctrl
}

func (s *MockBaseSuite) NewActionAPI(c *tc.C) *ActionAPI {
	modelUUID := modeltesting.GenModelUUID(c)

	s.Authorizer.EXPECT().AuthClient().Return(true)
	s.Authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	api, err := newActionAPI(s.Authorizer, LeaderFactory(s.Leadership), s.ApplicationService, s.BlockCommandService,
		s.ModelInfoService, s.OperationService, modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	return api
}

func NewActionAPI(
	authorizer facade.Authorizer,
	leadership leadership.Reader,
	applicationService ApplicationService,
	blockCommandService common.BlockCommandService,
	modelInfoService ModelInfoService,
	operationService OperationService,
	modelUUID coremodel.UUID,
) (*ActionAPI, error) {
	return newActionAPI(authorizer, LeaderFactory(leadership), applicationService, blockCommandService,
		modelInfoService, operationService, modelUUID)
}

type FakeLeadership struct {
	AppLeaders map[string]string
}

func (l FakeLeadership) Leaders() (map[string]string, error) {
	return l.AppLeaders, nil
}

func LeaderFactory(reader leadership.Reader) func() (leadership.Reader, error) {
	return func() (leadership.Reader, error) { return reader, nil }
}
