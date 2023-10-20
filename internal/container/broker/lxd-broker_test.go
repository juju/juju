// Copyright 2013-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker_test

import (
	stdcontext "context"
	"fmt"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/container/broker"
	"github.com/juju/juju/internal/container/broker/mocks"
	"github.com/juju/juju/internal/container/testing"
	coretools "github.com/juju/juju/internal/tools"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
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

	// To isolate the tests from the host's architecture, we override it here.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	broker.PatchNewMachineInitReader(s, newBlankMachineInitReader)

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
	return broker.NewLXDBroker(s.api.PrepareHost, s.api, s.manager, s.agentConfig)
}

func (s *lxdBrokerSuite) TestStartInstanceWithoutHostNetworkChanges(c *gc.C) {
	broker, brokerErr := s.newLXDBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)
	machineId := "1/lxd/0"
	containerTag := names.NewMachineTag("1-lxd-0")
	s.startInstance(c, broker, machineId)
	s.api.CheckCalls(c, []jujutesting.StubCall{{
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
	arch, err := instanceConfig.ToolsList().OneArch()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(arch, gc.Equals, "amd64")
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

	_, err := broker.StartInstance(context.WithoutCredentialInvalidator(stdcontext.Background()), environs.StartInstanceParams{
		Tools: coretools.List{{
			// non-host-arch tools should be filtered out by StartInstance
			Version: version.MustParseBinary("2.3.4-ubuntu-arm64"),
			URL:     "http://tools.testing.invalid/2.3.4-ubuntu-arm64.tgz",
		}},
		InstanceConfig: makeInstanceConfig(c, s, "1/lxd/0"),
	})
	c.Assert(err, gc.ErrorMatches, `need agent binaries for arch amd64, only found arm64`)
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
	broker.PatchNewMachineInitReader(s, newFakeMachineInitReader)
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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machineId := "1/lxd/0"
	containerTag := names.NewMachineTag("1-lxd-0")

	mockApi := mocks.NewMockAPICalls(ctrl)
	mockApi.EXPECT().PrepareContainerInterfaceInfo(gomock.Eq(containerTag)).Return(corenetwork.InterfaceInfos{fakeInterfaceInfo}, nil)
	mockApi.EXPECT().ContainerConfig().Return(fakeContainerConfig(), nil)

	put := lxdprofile.Profile{
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
	result := &apiprovisioner.LXDProfileResult{
		Config:  put.Config,
		Devices: put.Devices,
		Name:    "juju-test-profile",
	}
	mockApi.EXPECT().GetContainerProfileInfo(gomock.Eq(containerTag)).Return([]*apiprovisioner.LXDProfileResult{result}, nil)

	mockManager := testing.NewMockTestLXDManager(ctrl)
	mockManager.EXPECT().MaybeWriteLXDProfile("juju-test-profile", put).Return(nil)

	inst := mockInstance{id: "testinst"}
	arch := "testarch"
	hw := instance.HardwareCharacteristics{Arch: &arch}
	mockManager.EXPECT().CreateContainer(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(&inst, &hw, nil)

	broker, err := broker.NewLXDBroker(
		func(containerTag names.MachineTag, log loggo.Logger, abort <-chan struct{}) error { return nil },
		mockApi, mockManager, s.agentConfig)
	c.Assert(err, jc.ErrorIsNil)

	s.startInstance(c, broker, machineId)
}

func (s *lxdBrokerSuite) TestStartInstanceWithNoNameLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machineId := "1/lxd/0"
	containerTag := names.NewMachineTag("1-lxd-0")

	mockApi := mocks.NewMockAPICalls(ctrl)
	mockApi.EXPECT().PrepareContainerInterfaceInfo(gomock.Eq(containerTag)).Return(corenetwork.InterfaceInfos{fakeInterfaceInfo}, nil)
	mockApi.EXPECT().ContainerConfig().Return(fakeContainerConfig(), nil)

	put := &charm.LXDProfile{
		Config: map[string]string{
			"security.nesting": "true",
		},
	}
	result := &apiprovisioner.LXDProfileResult{
		Config: put.Config,
		Name:   "",
	}
	mockApi.EXPECT().GetContainerProfileInfo(gomock.Eq(containerTag)).Return([]*apiprovisioner.LXDProfileResult{result}, nil)

	mockManager := testing.NewMockTestLXDManager(ctrl)

	broker, err := broker.NewLXDBroker(
		func(containerTag names.MachineTag, log loggo.Logger, abort <-chan struct{}) error { return nil },
		mockApi, mockManager, s.agentConfig)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.startInstance(c, broker, machineId)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("cannot write charm profile: request to write LXD profile for machine %s with no profile name", machineId))
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

	broker, err := broker.NewLXDBroker(
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
