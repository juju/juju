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

//go:generate go run go.uber.org/mock/mockgen -typed -package spaces -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/spaces Backing,BlockChecker,Machine,Constraints,Address,NetworkService,ControllerConfigService,ApplicationService

// APISuite is used to test API calls using mocked model operations.
type APISuite struct {
	resource   *facademocks.MockResources
	authorizer *facademocks.MockAuthorizer

	Backing      *MockBacking
	blockChecker *MockBlockChecker

	// TODO (manadart 2020-03-24): Localise this to the suites that need it.
	Constraints *MockConstraints

	API *API

	ControllerConfigService *MockControllerConfigService
	NetworkService          *MockNetworkService
	ApplicationService      *MockApplicationService
}

func TestAPISuite(t *testing.T) {
	tc.Run(t, &APISuite{})
}

func (s *APISuite) TearDownTest(_ *tc.C) {
	s.API = nil
}

func (s *APISuite) SetupMocks(c *tc.C, supportSpaces bool, providerSpaces bool) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.resource = facademocks.NewMockResources(ctrl)
	s.Constraints = NewMockConstraints(ctrl)

	s.blockChecker = NewMockBlockChecker(ctrl)
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil).AnyTimes()

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	s.authorizer.EXPECT().AuthClient().Return(true)

	s.Backing = NewMockBacking(ctrl)

	s.ControllerConfigService = NewMockControllerConfigService(ctrl)
	s.NetworkService = NewMockNetworkService(ctrl)
	s.ApplicationService = NewMockApplicationService(ctrl)

	s.NetworkService.EXPECT().SupportsSpaces(gomock.Any()).Return(supportSpaces, nil).AnyTimes()
	s.NetworkService.EXPECT().SupportsSpaceDiscovery(gomock.Any()).Return(providerSpaces, nil).AnyTimes()

	var err error
	s.API, err = newAPIWithBacking(apiConfig{
		modelTag:                names.NewModelTag(modeltesting.GenModelUUID(c).String()),
		Backing:                 s.Backing,
		Check:                   s.blockChecker,
		Resources:               s.resource,
		Authorizer:              s.authorizer,
		ControllerConfigService: s.ControllerConfigService,
		NetworkService:          s.NetworkService,
		ApplicationService:      s.ApplicationService,
		logger:                  loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)

	return ctrl
}

// NewAPIWithBacking is also a legacy-only artifact,
// only used by the legacy test suite.
var NewAPIWithBacking = newAPIWithBacking

// APIConfig is also a legacy-only artifact.
type APIConfig = apiConfig
