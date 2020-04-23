// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"testing"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	environmocks "github.com/juju/juju/environs/mocks"
)

//go:generate mockgen -package spaces -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/spaces Backing,BlockChecker,Machine,RenameSpace,RenameSpaceState,Settings,OpFactory,RemoveSpace,Subnet,Constraints,MovingSubnet,MoveSubnetsOp,Address,ReloadSpaces,ReloadSpacesState,ReloadSpacesEnviron,EnvironSpaces,AuthorizerState

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

	cloudCallContext *context.CloudCallContext
	API              *API

	AuthorizerState     *MockAuthorizerState
	EnvironSpaces       *MockEnvironSpaces
	ReloadSpacesState   *MockReloadSpacesState
	ReloadSpacesEnviron *MockReloadSpacesEnviron
	ReloadSpacesAPI     *ReloadSpacesAPI
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) TearDownTest(_ *gc.C) {
	s.API = nil
}

func (s *APISuite) SetupMocks(c *gc.C, supportSpaces bool, providerSpaces bool) (*gomock.Controller, func()) {
	ctrl := gomock.NewController(c)

	s.resource = facademocks.NewMockResources(ctrl)
	s.cloudCallContext = context.NewCloudCallContext()
	s.OpFactory = NewMockOpFactory(ctrl)
	s.Constraints = NewMockConstraints(ctrl)

	s.blockChecker = NewMockBlockChecker(ctrl)
	s.blockChecker.EXPECT().ChangeAllowed().Return(nil).AnyTimes()

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	s.authorizer.EXPECT().AuthClient().Return(true)

	cloudSpec := environs.CloudSpec{
		Type:             "mock-provider",
		Name:             "cloud-name",
		Endpoint:         "endpoint",
		IdentityEndpoint: "identity-endpoint",
		StorageEndpoint:  "storage-endpoint",
	}

	s.Backing = NewMockBacking(ctrl)
	bExp := s.Backing.EXPECT()
	bExp.ModelTag().Return(names.NewModelTag("123"))
	bExp.ModelConfig().Return(nil, nil).AnyTimes()
	bExp.CloudSpec().Return(cloudSpec, nil).AnyTimes()

	mockNetworkEnviron := environmocks.NewMockNetworkingEnviron(ctrl)
	mockNetworkEnviron.EXPECT().SupportsSpaces(gomock.Any()).Return(supportSpaces, nil).AnyTimes()
	mockNetworkEnviron.EXPECT().SupportsSpaceDiscovery(gomock.Any()).Return(providerSpaces, nil).AnyTimes()

	mockProvider := environmocks.NewMockCloudEnvironProvider(ctrl)
	mockProvider.EXPECT().Open(gomock.Any()).Return(mockNetworkEnviron, nil).AnyTimes()

	unReg := environs.RegisterProvider("mock-provider", mockProvider)

	s.EnvironSpaces = NewMockEnvironSpaces(ctrl)
	s.ReloadSpacesState = NewMockReloadSpacesState(ctrl)
	s.ReloadSpacesEnviron = NewMockReloadSpacesEnviron(ctrl)
	s.AuthorizerState = NewMockAuthorizerState(ctrl)
	s.ReloadSpacesAPI = NewReloadSpacesAPI(
		s.ReloadSpacesState,
		s.ReloadSpacesEnviron,
		s.EnvironSpaces,
		s.cloudCallContext,
		DefaultReloadSpacesAuthorizer(
			s.authorizer,
			s.blockChecker,
			s.AuthorizerState,
		),
	)

	var err error
	s.API, err = newAPIWithBacking(apiConfig{
		ReloadSpacesAPI: s.ReloadSpacesAPI,
		Backing:         s.Backing,
		Check:           s.blockChecker,
		Context:         s.cloudCallContext,
		Resources:       s.resource,
		Authorizer:      s.authorizer,
		Factory:         s.OpFactory,
	})
	c.Assert(err, jc.ErrorIsNil)

	return ctrl, unReg
}

// SupportsSpaces is used by the legacy test suite and
// can be removed when it is grandfathered out.
func SupportsSpaces(backing Backing, ctx context.ProviderCallContext) error {
	api := &API{
		backing: backing,
		context: ctx,
	}
	return api.checkSupportsSpaces()
}

// NewAPIWithBacking is also a legacy-only artifact,
// only used by the legacy test suite.
var NewAPIWithBacking = newAPIWithBacking

// APIConfig is also a legacy-only artifact.
type APIConfig = apiConfig
