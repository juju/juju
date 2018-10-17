// Copyright 2013-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"runtime"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/testing"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/provisioner"
	"github.com/juju/juju/worker/provisioner/mocks"
)

type lxdBrokerSuite struct {
	coretesting.BaseSuite
	agentConfig agent.ConfigSetterWriter
	api         *fakeAPI
	manager     *fakeContainerManager
}

var _ = gc.Suite(&lxdBrokerSuite{})

func (s *lxdBrokerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	if runtime.GOOS == "windows" {
		c.Skip("Skipping lxd tests on windows")
	}

	// To isolate the tests from the host's architecture, we override it here.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	s.PatchValue(&provisioner.GetMachineCloudInitData, func(_ string) (map[string]interface{}, error) {
		return nil, nil
	})

	var err error
	s.agentConfig, err = agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             agent.NewPathsWithDefaults(agent.Paths{DataDir: "/not/used/here"}),
			Tag:               names.NewMachineTag("1"),
			UpgradedToVersion: jujuversion.Current,
			Password:          "dummy-secret",
			Nonce:             "nonce",
			APIAddresses:      []string{"10.0.0.1:1234"},
			CACert:            coretesting.CACert,
			Controller:        coretesting.ControllerTag,
			Model:             coretesting.ModelTag,
		})
	c.Assert(err, jc.ErrorIsNil)
	s.api = NewFakeAPI()
	s.manager = &fakeContainerManager{}
}

func (s *lxdBrokerSuite) startInstance(c *gc.C, broker environs.InstanceBroker, machineId string) (*environs.StartInstanceResult, error) {
	return callStartInstance(c, s, broker, machineId)
}

func (s *lxdBrokerSuite) newLXDBroker(c *gc.C) (environs.InstanceBroker, error) {
	return provisioner.NewLXDBroker(s.api.PrepareHost, s.api, s.manager, s.agentConfig)
}

func (s *lxdBrokerSuite) TestStartInstanceWithoutHostNetworkChanges(c *gc.C) {
	broker, brokerErr := s.newLXDBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)
	machineId := "1/lxd/0"
	containerTag := names.NewMachineTag("1-lxd-0")
	s.startInstance(c, broker, machineId)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "ContainerConfig",
	}, {
		FuncName: "PrepareHost",
		Args:     []interface{}{containerTag},
	}, {
		FuncName: "PrepareContainerInterfaceInfo",
		Args:     []interface{}{names.NewMachineTag("1-lxd-0")},
	}, {
		FuncName: "GetContainerProfileInfo",
		Args:     []interface{}{names.NewMachineTag("1-lxd-0")},
	}})
	s.manager.CheckCallNames(c, "CreateContainer")
	call := s.manager.Calls()[0]
	c.Assert(call.Args[0], gc.FitsTypeOf, &instancecfg.InstanceConfig{})
	instanceConfig := call.Args[0].(*instancecfg.InstanceConfig)
	c.Assert(instanceConfig.ToolsList(), gc.HasLen, 1)
	c.Assert(instanceConfig.ToolsList().Arches(), jc.DeepEquals, []string{"amd64"})
}

func (s *lxdBrokerSuite) TestStartInstancePopulatesNetworkInfo(c *gc.C) {
	broker, brokerErr := s.newLXDBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	patchResolvConf(s, c)

	result, err := s.startInstance(c, broker, "1/lxd/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.NetworkInfo, gc.HasLen, 1)
	iface := result.NetworkInfo[0]
	c.Assert(iface, jc.DeepEquals, network.InterfaceInfo{
		DeviceIndex:         0,
		CIDR:                "0.1.2.0/24",
		InterfaceName:       "dummy0",
		ParentInterfaceName: "lxdbr0",
		MACAddress:          "aa:bb:cc:dd:ee:ff",
		Address:             network.NewAddress("0.1.2.3"),
		GatewayAddress:      network.NewAddress("0.1.2.1"),
		DNSServers:          network.NewAddresses("ns1.dummy", "ns2.dummy"),
		DNSSearchDomains:    []string{"dummy", "invalid"},
	})
}

func (s *lxdBrokerSuite) TestStartInstancePopulatesFallbackNetworkInfo(c *gc.C) {
	broker, brokerErr := s.newLXDBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	patchResolvConf(s, c)

	s.api.SetErrors(
		nil, // ContainerConfig succeeds
		nil, // HostChangesForContainer succeeds
		errors.NotSupportedf("container address allocation"),
	)
	_, err := s.startInstance(c, broker, "1/lxd/0")
	c.Assert(err, gc.ErrorMatches, "container address allocation not supported")
}

func (s *lxdBrokerSuite) TestStartInstanceNoHostArchTools(c *gc.C) {
	broker, brokerErr := s.newLXDBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	_, err := broker.StartInstance(context.NewCloudCallContext(), environs.StartInstanceParams{
		Tools: coretools.List{{
			// non-host-arch tools should be filtered out by StartInstance
			Version: version.MustParseBinary("2.3.4-quantal-arm64"),
			URL:     "http://tools.testing.invalid/2.3.4-quantal-arm64.tgz",
		}},
		InstanceConfig: makeInstanceConfig(c, s, "1/lxd/0"),
	})
	c.Assert(err, gc.ErrorMatches, `need agent binaries for arch amd64, only found \[arm64\]`)
}

func (s *lxdBrokerSuite) TestStartInstanceWithCloudInitUserData(c *gc.C) {
	broker, brokerErr := s.newLXDBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	_, err := s.startInstance(c, broker, "1/lxd/0")
	c.Assert(err, jc.ErrorIsNil)

	s.manager.CheckCallNames(c, "CreateContainer")
	call := s.manager.Calls()[0]
	c.Assert(call.Args[0], gc.FitsTypeOf, &instancecfg.InstanceConfig{})
	instanceConfig := call.Args[0].(*instancecfg.InstanceConfig)
	assertCloudInitUserData(instanceConfig.CloudInitUserData, map[string]interface{}{
		"packages":        []interface{}{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []interface{}{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []interface{}{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
		"package_upgrade": false,
	}, c)
}

func (s *lxdBrokerSuite) TestStartInstanceWithContainerInheritProperties(c *gc.C) {
	s.PatchValue(&provisioner.GetMachineCloudInitData, func(_ string) (map[string]interface{}, error) {
		return map[string]interface{}{
			"packages":   []interface{}{"python-novaclient"},
			"fake-entry": []interface{}{"testing-garbage"},
			"apt": map[interface{}]interface{}{
				"primary": []interface{}{
					map[interface{}]interface{}{
						"arches": []interface{}{"default"},
						"uri":    "http://archive.ubuntu.com/ubuntu",
					},
				},
				"security": []interface{}{
					map[interface{}]interface{}{
						"arches": []interface{}{"default"},
						"uri":    "http://archive.ubuntu.com/ubuntu",
					},
				},
			},
			"ca-certs": map[interface{}]interface{}{
				"remove-defaults": true,
				"trusted":         []interface{}{"-----BEGIN CERTIFICATE-----\nYOUR-ORGS-TRUSTED-CA-CERT-HERE\n-----END CERTIFICATE-----\n"},
			},
		}, nil
	})
	s.api.fakeContainerConfig.ContainerInheritProperties = "ca-certs,apt-security"

	broker, brokerErr := s.newLXDBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)
	_, err := s.startInstance(c, broker, "1/lxd/0")
	c.Assert(err, jc.ErrorIsNil)

	s.manager.CheckCallNames(c, "CreateContainer")
	call := s.manager.Calls()[0]
	c.Assert(call.Args[0], gc.FitsTypeOf, &instancecfg.InstanceConfig{})
	instanceConfig := call.Args[0].(*instancecfg.InstanceConfig)
	assertCloudInitUserData(instanceConfig.CloudInitUserData, map[string]interface{}{
		"packages":        []interface{}{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []interface{}{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []interface{}{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
		"package_upgrade": false,
		"apt": map[string]interface{}{
			"security": []interface{}{
				map[interface{}]interface{}{
					"arches": []interface{}{"default"},
					"uri":    "http://archive.ubuntu.com/ubuntu",
				},
			},
		},
		"ca-certs": map[interface{}]interface{}{
			"remove-defaults": true,
			"trusted": []interface{}{
				"-----BEGIN CERTIFICATE-----\nYOUR-ORGS-TRUSTED-CA-CERT-HERE\n-----END CERTIFICATE-----\n"},
		},
	}, c)
}

func (s *lxdBrokerSuite) TestStartInstanceWithLXDProfile(c *gc.C) {
	ctlr := gomock.NewController(c)
	defer ctlr.Finish()

	machineId := "1/lxd/0"
	containerTag := names.NewMachineTag("1-lxd-0")

	mockApi := mocks.NewMockAPICalls(ctlr)
	mockApi.EXPECT().PrepareContainerInterfaceInfo(gomock.Eq(containerTag)).Return([]network.InterfaceInfo{fakeInterfaceInfo}, nil)
	mockApi.EXPECT().ContainerConfig().Return(fakeContainerConfig(), nil)

	put := &charm.LXDProfile{
		Config: map[string]string{
			"security.nesting": "true",
		},
		Devices: map[string]map[string]string{
			"bdisk": {
				"source": "/dev/loop0",
				"type":   "unix-block",
			},
		},
	}
	result := apiprovisioner.LXDProfileResult{
		Config:  put.Config,
		Devices: put.Devices,
		Name:    "juju-test-profile",
	}
	mockApi.EXPECT().GetContainerProfileInfo(gomock.Eq(containerTag)).Return([]apiprovisioner.LXDProfileResult{result}, nil)

	mockManager := testing.NewMockTestLXDManager(ctlr)
	mockManager.EXPECT().MaybeWriteLXDProfile("juju-test-profile", gomock.Eq(put)).Return(nil)

	inst := mockInstance{id: "testinst"}
	arch := "testarch"
	hw := instance.HardwareCharacteristics{Arch: &arch}
	mockManager.EXPECT().CreateContainer(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(&inst, &hw, nil)

	broker, err := provisioner.NewLXDBroker(
		func(containerTag names.MachineTag, log loggo.Logger, abort <-chan struct{}) error { return nil },
		mockApi, mockManager, s.agentConfig)
	c.Assert(err, jc.ErrorIsNil)

	s.startInstance(c, broker, machineId)
}

func (s *lxdBrokerSuite) TestStartInstanceWithLXDProfileReturnsLXDProfileNames(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	containerTag := names.NewMachineTag("1-lxd-0")

	mockApi := mocks.NewMockAPICalls(ctrl)
	mockManager := testing.NewMockTestLXDManager(ctrl)
	mockManager.EXPECT().LXDProfileNames(containerTag.Id()).Return([]string{
		lxdprofile.Name("foo", "bar", 1),
	}, nil)

	broker, err := provisioner.NewLXDBroker(
		func(containerTag names.MachineTag, log loggo.Logger, abort <-chan struct{}) error { return nil },
		mockApi, mockManager, s.agentConfig)
	c.Assert(err, jc.ErrorIsNil)

	nameRetriever := broker.(container.LXDProfileNameRetriever)
	profileNames, err := nameRetriever.LXDProfileNames(containerTag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(profileNames, jc.DeepEquals, []string{
		lxdprofile.Name("foo", "bar", 1),
	})
}
