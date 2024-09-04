// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"testing"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package spaces -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/spaces Backing,BlockChecker,Machine,Constraints,Address,Unit,Bindings,NetworkService,ControllerConfigService

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

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
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) TearDownTest(_ *gc.C) {
	s.API = nil
}

func (s *APISuite) SetupMocks(c *gc.C, supportSpaces bool, providerSpaces bool) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.resource = facademocks.NewMockResources(ctrl)
	s.Constraints = NewMockConstraints(ctrl)

	s.blockChecker = NewMockBlockChecker(ctrl)
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil).AnyTimes()

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	s.authorizer.EXPECT().AuthClient().Return(true)

	s.Backing = NewMockBacking(ctrl)
	bExp := s.Backing.EXPECT()
	bExp.ModelTag().Return(names.NewModelTag("123"))

	s.ControllerConfigService = NewMockControllerConfigService(ctrl)
	s.NetworkService = NewMockNetworkService(ctrl)

	s.NetworkService.EXPECT().SupportsSpaces(gomock.Any()).Return(supportSpaces, nil).AnyTimes()
	s.NetworkService.EXPECT().SupportsSpaceDiscovery(gomock.Any(), gomock.Any()).Return(providerSpaces, nil).AnyTimes()

	var err error
	s.API, err = newAPIWithBacking(apiConfig{
		Backing:                     s.Backing,
		Check:                       s.blockChecker,
		CredentialInvalidatorGetter: apiservertesting.NoopModelCredentialInvalidatorGetter,
		Resources:                   s.resource,
		Authorizer:                  s.authorizer,
		ControllerConfigService:     s.ControllerConfigService,
		NetworkService:              s.NetworkService,
		logger:                      loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

// NewAPIWithBacking is also a legacy-only artifact,
// only used by the legacy test suite.
var NewAPIWithBacking = newAPIWithBacking

// APIConfig is also a legacy-only artifact.
type APIConfig = apiConfig
