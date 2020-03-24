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
	"github.com/juju/juju/apiserver/facades/client/spaces/mocks"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	environmocks "github.com/juju/juju/environs/mocks"
)

//go:generate mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/apiserver/facades/client/spaces Backing,BlockChecker,Machine,RenameSpace,RenameSpaceState,Settings,OpFactory,RemoveSpace,Subnet,Constraints,MovingSubnet,MoveSubnetsOp,Address

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

// APISuite is used to test API calls using mocked model operations.
type APISuite struct {
	resource   *facademocks.MockResources
	authorizer *facademocks.MockAuthorizer

	Backing      *mocks.MockBacking
	blockChecker *mocks.MockBlockChecker

	// TODO (manadart 2020-03-24): Localise this to the suites that need it.
	constraints *mocks.MockConstraints
	OpFactory   *mocks.MockOpFactory

	cloudCallContext *context.CloudCallContext
	API              *API
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) TearDownTest(_ *gc.C) {
	s.API = nil
}

func (s *APISuite) SetupMocks(c *gc.C, supportSpaces bool, providerSpaces bool) (*gomock.Controller, func()) {
	ctrl := gomock.NewController(c)

	s.resource = facademocks.NewMockResources(ctrl)
	s.cloudCallContext = context.NewCloudCallContext()
	s.OpFactory = mocks.NewMockOpFactory(ctrl)
	s.constraints = mocks.NewMockConstraints(ctrl)

	s.blockChecker = mocks.NewMockBlockChecker(ctrl)
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

	s.Backing = mocks.NewMockBacking(ctrl)
	bExp := s.Backing.EXPECT()
	bExp.ModelTag().Return(names.NewModelTag("123"))
	bExp.ModelConfig().Return(nil, nil).AnyTimes()
	bExp.CloudSpec().Return(cloudSpec, nil).AnyTimes()

	mockNetworkEnviron := environmocks.NewMockNetworkingEnviron(ctrl)
	mockNetworkEnviron.EXPECT().SupportsSpaces(gomock.Any()).Return(supportSpaces, nil).AnyTimes()
	mockNetworkEnviron.EXPECT().SupportsProviderSpaces(gomock.Any()).Return(providerSpaces, nil).AnyTimes()

	mockProvider := environmocks.NewMockCloudEnvironProvider(ctrl)
	mockProvider.EXPECT().Open(gomock.Any()).Return(mockNetworkEnviron, nil).AnyTimes()

	unReg := environs.RegisterProvider("mock-provider", mockProvider)

	var err error
	s.API, err = newAPIWithBacking(
		s.Backing, s.blockChecker, s.cloudCallContext, s.resource, s.authorizer, s.OpFactory)
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
