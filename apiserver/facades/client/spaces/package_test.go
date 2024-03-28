// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	stdcontext "context"
	"testing"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	environmocks "github.com/juju/juju/environs/mocks"
)

//go:generate go run go.uber.org/mock/mockgen -package spaces -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/spaces Backing,BlockChecker,Machine,RenameSpace,RenameSpaceState,Settings,OpFactory,RemoveSpace,Constraints,Address,Unit,ReloadSpaces,ReloadSpacesState,ReloadSpacesEnviron,EnvironSpaces,AuthorizerState,Bindings,SpaceService,SubnetService,ControllerConfigService

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
	OpFactory   *MockOpFactory

	API *API

	AuthorizerState         *MockAuthorizerState
	EnvironSpaces           *MockEnvironSpaces
	ReloadSpacesState       *MockReloadSpacesState
	ReloadSpacesEnviron     *MockReloadSpacesEnviron
	ReloadSpacesAPI         *ReloadSpacesAPI
	ControllerConfigService *MockControllerConfigService
	SpaceService            *MockSpaceService
	SubnetService           *MockSubnetService
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) TearDownTest(_ *gc.C) {
	s.API = nil
}

func (s *APISuite) SetupMocks(c *gc.C, supportSpaces bool, providerSpaces bool) (*gomock.Controller, func()) {
	ctrl := gomock.NewController(c)

	s.resource = facademocks.NewMockResources(ctrl)
	s.OpFactory = NewMockOpFactory(ctrl)
	s.Constraints = NewMockConstraints(ctrl)

	s.blockChecker = NewMockBlockChecker(ctrl)
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil).AnyTimes()

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	s.authorizer.EXPECT().AuthClient().Return(true)

	cloudSpec := environscloudspec.CloudSpec{
		Type:             "mock-provider",
		Name:             "cloud-name",
		Endpoint:         "endpoint",
		IdentityEndpoint: "identity-endpoint",
		StorageEndpoint:  "storage-endpoint",
	}

	s.Backing = NewMockBacking(ctrl)
	bExp := s.Backing.EXPECT()
	bExp.ModelTag().Return(names.NewModelTag("123"))
	bExp.ModelConfig(gomock.Any()).Return(nil, nil).AnyTimes()
	bExp.CloudSpec(gomock.Any()).Return(cloudSpec, nil).AnyTimes()

	mockNetworkEnviron := environmocks.NewMockNetworkingEnviron(ctrl)
	mockNetworkEnviron.EXPECT().SupportsSpaces(gomock.Any()).Return(supportSpaces, nil).AnyTimes()
	mockNetworkEnviron.EXPECT().SupportsSpaceDiscovery(gomock.Any()).Return(providerSpaces, nil).AnyTimes()

	mockProvider := environmocks.NewMockCloudEnvironProvider(ctrl)
	mockProvider.EXPECT().Open(gomock.Any(), gomock.Any()).Return(mockNetworkEnviron, nil).AnyTimes()

	unReg := environs.RegisterProvider("mock-provider", mockProvider)

	s.EnvironSpaces = NewMockEnvironSpaces(ctrl)
	s.ReloadSpacesState = NewMockReloadSpacesState(ctrl)
	s.ReloadSpacesEnviron = NewMockReloadSpacesEnviron(ctrl)
	s.AuthorizerState = NewMockAuthorizerState(ctrl)
	s.ReloadSpacesAPI = NewReloadSpacesAPI(
		s.ReloadSpacesState,
		s.ReloadSpacesEnviron,
		s.EnvironSpaces,
		apiservertesting.NoopModelCredentialInvalidatorGetter,
		DefaultReloadSpacesAuthorizer(
			s.authorizer,
			s.blockChecker,
			s.AuthorizerState,
		),
	)
	s.ControllerConfigService = NewMockControllerConfigService(ctrl)
	s.SpaceService = NewMockSpaceService(ctrl)
	s.SubnetService = NewMockSubnetService(ctrl)

	var err error
	s.API, err = newAPIWithBacking(apiConfig{
		ReloadSpacesAPI:             s.ReloadSpacesAPI,
		Backing:                     s.Backing,
		Check:                       s.blockChecker,
		CredentialInvalidatorGetter: apiservertesting.NoopModelCredentialInvalidatorGetter,
		Resources:                   s.resource,
		Authorizer:                  s.authorizer,
		Factory:                     s.OpFactory,
		ControllerConfigService:     s.ControllerConfigService,
		SpaceService:                s.SpaceService,
		SubnetService:               s.SubnetService,
	})
	c.Assert(err, jc.ErrorIsNil)

	return ctrl, unReg
}

// SupportsSpaces is used by the legacy test suite and
// can be removed when it is grandfathered out.
func SupportsSpaces(backing Backing) error {
	api := &API{
		backing:                     backing,
		credentialInvalidatorGetter: apiservertesting.NoopModelCredentialInvalidatorGetter,
	}
	return api.checkSupportsSpaces(stdcontext.Background())
}

// NewAPIWithBacking is also a legacy-only artifact,
// only used by the legacy test suite.
var NewAPIWithBacking = newAPIWithBacking

// APIConfig is also a legacy-only artifact.
type APIConfig = apiConfig
