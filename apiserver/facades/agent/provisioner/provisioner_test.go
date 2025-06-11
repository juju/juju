// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/environs/config"
	environtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

// TODO(jam): 2017-02-15 We seem to be lacking most of direct unit tests around ProcessOneContainer
// Some of the use cases we need to be testing are:
// 1) Provider can allocate addresses, should result in a container with
//    addresses requested from the provider, and 'static' configuration on those
//    devices.
// 2) Provider cannot allocate addresses, currently this should make us use
//    'lxdbr0' and DHCP allocated addresses.
// 3) Provider could allocate DHCP based addresses on the host device, which would let us
//    use a bridge on the device and DHCP. (Currently not supported, but desirable for
//    vSphere and Manual and probably LXD providers.)
// Addition (manadart 2018-10-09): To begin accommodating the deficiencies noted
// above, the new suite below uses mocks for tests ill-suited to the dummy
// provider. We could reasonably re-write the tests above over time to use the
// new suite.
// Addition (tlm 2024-08-27): The old "integration" tests using apiserver suite
// have been put into their own file. New tests should be added here using
// mocks.

type provisionerMockSuite struct {
	coretesting.BaseSuite

	environ            *environtesting.MockNetworkingEnviron
	policy             *MockBridgePolicy
	host               *MockMachine
	container          *MockMachine
	applicationService *MockApplicationService
	device             *MockLinkLayerDevice
	parentDevice       *MockLinkLayerDevice

	unit        *MockUnit
	application *MockApplication
}

func TestProvisionerMockSuite(t *testing.T) {
	tc.Run(t, &provisionerMockSuite{})
}

// Even when the provider supports container addresses, manually provisioned
// machines should fall back to DHCP.
func (s *provisionerMockSuite) TestManuallyProvisionedHostsUseDHCPForContainers(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectManuallyProvisionedHostsUseDHCPForContainers()

	res := params.MachineNetworkConfigResults{
		Results: []params.MachineNetworkConfigResult{{}},
	}
	ctx := prepareOrGetHandler{result: res, maintain: false, logger: loggertesting.WrapCheckLog(c)}

	// ProviderCallContext is not required by this logical path and can be nil
	err := ctx.ProcessOneContainer(c.Context(), s.environ, s.policy, 0, s.host, s.container, "", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results[0].Config, tc.HasLen, 1)

	cfg := res.Results[0].Config[0]
	c.Check(cfg.ConfigType, tc.Equals, "dhcp")
	c.Check(cfg.ProviderSubnetId, tc.Equals, "")
	c.Check(cfg.VLANTag, tc.Equals, 0)
}

func (s *provisionerMockSuite) expectManuallyProvisionedHostsUseDHCPForContainers() {
	s.expectNetworkingEnviron()

	cExp := s.container.EXPECT()

	s.policy.EXPECT().PopulateContainerLinkLayerDevices(s.host, s.container, false).Return(
		network.InterfaceInfos{
			{
				InterfaceName: "eth0",
				ConfigType:    network.ConfigDHCP,
			},
		}, nil)

	cExp.Id().Return("lxd/0").AnyTimes()

	hExp := s.host.EXPECT()
	// Crucial behavioural trait. Set false to test failure, whereupon the
	// PopulateContainerLinkLayerDevices expectation will not be satisfied.
	hExp.IsManual().Return(true, nil)
}

// expectNetworkingEnviron stubs an environ that supports container networking.
func (s *provisionerMockSuite) expectNetworkingEnviron() {
	eExp := s.environ.EXPECT()
	eExp.Config().Return(&config.Config{}).AnyTimes()
	eExp.SupportsContainerAddresses(gomock.Any()).Return(true, nil).AnyTimes()
}

func (s *provisionerMockSuite) TestContainerAlreadyProvisionedError(c *tc.C) {
	defer s.setup(c).Finish()

	exp := s.container.EXPECT()
	exp.Id().Return("0/lxd/0")

	res := params.MachineNetworkConfigResults{
		Results: []params.MachineNetworkConfigResult{{}},
	}
	ctx := prepareOrGetHandler{
		result:   res,
		maintain: true,
		logger:   loggertesting.WrapCheckLog(c),
	}
	// ProviderCallContext and BridgePolicy are not
	// required by this logical path and can be nil.
	err := ctx.ProcessOneContainer(c.Context(), s.environ, nil, 0, s.host, s.container, "", instance.Id("juju-8ebd6c-0"), nil)
	c.Assert(err, tc.ErrorMatches, `container "0/lxd/0" already provisioned as "juju-8ebd6c-0"`)
}

// TODO: this is not a great test name, this test does not even call
//
//	ProvisionerAPI.GetContainerProfileInfo.
func (s *provisionerMockSuite) TestGetContainerProfileInfo(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()
	s.expectCharmLXDProfiles(ctrl)

	s.application.EXPECT().Name().Return("application")

	locator := applicationcharm.CharmLocator{
		Name:     "application",
		Revision: 42,
		Source:   applicationcharm.CharmHubSource,
	}
	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "application").Return(locator, nil)
	s.applicationService.EXPECT().GetCharmLXDProfile(gomock.Any(), locator).Return(charm.LXDProfile{
		Config: map[string]string{
			"security.nesting":    "true",
			"security.privileged": "true",
		},
	}, 3, nil)

	res := params.ContainerProfileResults{
		Results: []params.ContainerProfileResult{{}},
	}
	ctx := containerProfileHandler{
		applicationService: s.applicationService,
		result:             res,
		modelName:          "testme",
		logger:             loggertesting.WrapCheckLog(c),
	}
	// ProviderCallContext and BridgePolicy are not
	// required by this logical path and can be nil.
	err := ctx.ProcessOneContainer(c.Context(), s.environ, nil, 0, s.host, s.container, "", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.IsNil)
	c.Assert(res.Results[0].LXDProfiles, tc.HasLen, 1)
	profile := res.Results[0].LXDProfiles[0]
	c.Check(profile.Name, tc.Equals, "juju-testme-application-3")
	c.Check(profile.Profile.Config, tc.DeepEquals,
		map[string]string{
			"security.nesting":    "true",
			"security.privileged": "true",
		},
	)
}

func (s *provisionerMockSuite) TestGetContainerProfileInfoNoProfile(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()
	s.expectCharmLXDProfiles(ctrl)

	s.unit.EXPECT().Name().Return("application/0")
	s.application.EXPECT().Name().Return("application")

	locator := applicationcharm.CharmLocator{
		Name:     "application",
		Revision: 42,
		Source:   applicationcharm.CharmHubSource,
	}
	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "application").Return(locator, nil)
	s.applicationService.EXPECT().GetCharmLXDProfile(gomock.Any(), locator).Return(charm.LXDProfile{}, -1, nil)

	res := params.ContainerProfileResults{
		Results: []params.ContainerProfileResult{{}},
	}
	ctx := containerProfileHandler{
		applicationService: s.applicationService,
		result:             res,
		modelName:          "testme",
		logger:             loggertesting.WrapCheckLog(c),
	}
	// ProviderCallContext and BridgePolicy are not
	// required by this logical path and can be nil.
	err := ctx.ProcessOneContainer(c.Context(), s.environ, nil, 0, s.host, s.container, "", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.IsNil)
	c.Assert(res.Results[0].LXDProfiles, tc.HasLen, 0)
}

func (s *provisionerMockSuite) expectCharmLXDProfiles(ctrl *gomock.Controller) {
	s.container.EXPECT().Units().Return([]Unit{s.unit}, nil)
	s.unit.EXPECT().Application().Return(s.application, nil)
}

func (s *provisionerMockSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.environ = environtesting.NewMockNetworkingEnviron(ctrl)
	s.policy = NewMockBridgePolicy(ctrl)
	s.host = NewMockMachine(ctrl)
	s.container = NewMockMachine(ctrl)
	s.device = NewMockLinkLayerDevice(ctrl)
	s.parentDevice = NewMockLinkLayerDevice(ctrl)

	s.applicationService = NewMockApplicationService(ctrl)
	s.application = NewMockApplication(ctrl)
	s.unit = NewMockUnit(ctrl)

	return ctrl
}

func TestWithControllerSuite(t *testing.T) {
	tc.Run(t, &withControllerSuite{})
}

type withControllerSuite struct {
	apiAddressAccessor      *MockAPIAddressAccessor
	controllerConfigService *MockControllerConfigService
}

func (s *withControllerSuite) TestAPIAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	s.apiAddressAccessor.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(map[string][]string{
		"1": {
			"0.1.2.3:1234",
		},
	}, nil)
	provisioner := &ProvisionerAPI{APIAddresser: common.NewAPIAddresser(s.apiAddressAccessor, nil)}

	// Act
	result, err := provisioner.APIAddresses(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	})
}

func (s *withControllerSuite) TestCACert(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(coretesting.FakeControllerConfig(), nil)
	provisioner := &ProvisionerAPI{controllerConfigService: s.controllerConfigService}

	// Act
	result, err := provisioner.CACert(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.BytesResult{
		Result: []byte(coretesting.CACert),
	})
}

func (s *withControllerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiAddressAccessor = NewMockAPIAddressAccessor(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)

	c.Cleanup(func() {
		s.apiAddressAccessor = nil
		s.controllerConfigService = nil
	})

	return ctrl
}
