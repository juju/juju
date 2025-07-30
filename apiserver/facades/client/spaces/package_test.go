// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	modeltesting "github.com/juju/juju/core/model/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package spaces -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/spaces BlockChecker,Constraints,NetworkService,ControllerConfigService,ApplicationService,MachineService

// APIBaseSuite is used to test API calls using mocked model operations.
type APIBaseSuite struct {
	resource   *facademocks.MockResources
	authorizer *facademocks.MockAuthorizer

	blockChecker *MockBlockChecker

	// TODO (manadart 2020-03-24): Localise this to the suites that need it.
	Constraints *MockConstraints

	API *API

	ControllerConfigService *MockControllerConfigService
	NetworkService          *MockNetworkService
	ApplicationService      *MockApplicationService
	MachineService          *MockMachineService
}

func TestAPISuite(t *testing.T) {
	tc.Run(t, &APIBaseSuite{})
}

func (s *APIBaseSuite) TearDownTest(_ *tc.C) {
	s.API = nil
}

func (s *APIBaseSuite) SetupMocks(c *tc.C, supportSpaces bool, providerSpaces bool) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.resource = facademocks.NewMockResources(ctrl)
	s.Constraints = NewMockConstraints(ctrl)

	s.blockChecker = NewMockBlockChecker(ctrl)
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil).AnyTimes()

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	s.ControllerConfigService = NewMockControllerConfigService(ctrl)
	s.NetworkService = NewMockNetworkService(ctrl)
	s.ApplicationService = NewMockApplicationService(ctrl)
	s.MachineService = NewMockMachineService(ctrl)

	s.NetworkService.EXPECT().SupportsSpaces(gomock.Any()).Return(supportSpaces, nil).AnyTimes()
	s.NetworkService.EXPECT().SupportsSpaceDiscovery(gomock.Any()).Return(providerSpaces, nil).AnyTimes()

	s.API = &API{
		modelTag:                names.NewModelTag(modeltesting.GenModelUUID(c).String()),
		check:                   s.blockChecker,
		auth:                    s.authorizer,
		controllerConfigService: s.ControllerConfigService,
		networkService:          s.NetworkService,
		applicationService:      s.ApplicationService,
		machineService:          s.MachineService,
		logger:                  loggertesting.WrapCheckLog(c),
	}

	return ctrl
}
