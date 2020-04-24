// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker_test

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/broker"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/container/kvm/mock"
	kvmtesting "github.com/juju/juju/container/kvm/testing"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type kvmSuite struct {
	kvmtesting.TestSuite
	events     chan mock.Event
	eventsDone chan struct{}
}

type kvmBrokerSuite struct {
	kvmSuite
	agentConfig agent.Config
	api         *fakeAPI
	manager     *fakeContainerManager
}

var _ = gc.Suite(&kvmBrokerSuite{})

func (s *kvmSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping kvm tests on windows")
	}
	s.TestSuite.SetUpTest(c)
	s.events = make(chan mock.Event)
	s.eventsDone = make(chan struct{})
	go func() {
		defer close(s.eventsDone)
		for event := range s.events {
			c.Output(3, fmt.Sprintf("kvm event: <%s, %s>", event.Action, event.InstanceId))
		}
	}()
	s.TestSuite.ContainerFactory.AddListener(s.events)
}

func (s *kvmSuite) TearDownTest(c *gc.C) {
	close(s.events)
	<-s.eventsDone
	s.TestSuite.TearDownTest(c)
}

func (s *kvmBrokerSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping kvm tests on windows")
	}
	s.kvmSuite.SetUpTest(c)
	broker.PatchNewMachineInitReader(s, newBlankMachineInitReader)

	var err error
	s.agentConfig, err = agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             agent.NewPathsWithDefaults(agent.Paths{DataDir: "/not/used/here"}),
			Tag:               names.NewUnitTag("ubuntu/1"),
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

func (s *kvmBrokerSuite) startInstance(c *gc.C, broker environs.InstanceBroker, machineId string) (*environs.StartInstanceResult, error) {
	return callStartInstance(c, s, broker, machineId)
}

func (s *kvmBrokerSuite) newKVMBroker(c *gc.C) (environs.InstanceBroker, error) {
	managerConfig := container.ManagerConfig{container.ConfigModelUUID: coretesting.ModelTag.Id()}
	manager, err := kvm.NewContainerManager(managerConfig)
	c.Assert(err, jc.ErrorIsNil)
	return broker.NewKVMBroker(s.api.PrepareHost, s.api, manager, s.agentConfig)
}

func (s *kvmBrokerSuite) newKVMBrokerFakeManager(c *gc.C) (environs.InstanceBroker, error) {
	return broker.NewKVMBroker(s.api.PrepareHost, s.api, s.manager, s.agentConfig)
}

func (s *kvmBrokerSuite) maintainInstance(c *gc.C, broker environs.InstanceBroker, machineId string) {
	callMaintainInstance(c, s, broker, machineId)
}

func (s *kvmBrokerSuite) TestStartInstanceWithoutNetworkChanges(c *gc.C) {
	broker, brokerErr := s.newKVMBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	machineId := "1/kvm/0"
	result, err := s.startInstance(c, broker, machineId)
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "ContainerConfig",
	}, {
		FuncName: "PrepareHost",
		Args:     []interface{}{names.NewMachineTag("1-kvm-0")},
	}, {
		FuncName: "PrepareContainerInterfaceInfo",
		Args:     []interface{}{names.NewMachineTag("1-kvm-0")},
	}})
	c.Assert(result.Instance.Id(), gc.Equals, instance.Id("juju-06f00d-1-kvm-0"))
	s.assertResults(c, broker, result)
}

func (s *kvmBrokerSuite) TestMaintainInstanceAddress(c *gc.C) {
	broker, brokerErr := s.newKVMBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	machineId := "1/kvm/0"
	result, err := s.startInstance(c, broker, machineId)
	c.Assert(err, jc.ErrorIsNil)

	s.api.ResetCalls()

	s.maintainInstance(c, broker, machineId)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{})
	c.Assert(result.Instance.Id(), gc.Equals, instance.Id("juju-06f00d-1-kvm-0"))
	s.assertResults(c, broker, result)
}

func (s *kvmBrokerSuite) TestStopInstance(c *gc.C) {
	broker, brokerErr := s.newKVMBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	result0, err0 := s.startInstance(c, broker, "1/kvm/0")
	c.Assert(err0, jc.ErrorIsNil)

	result1, err1 := s.startInstance(c, broker, "1/kvm/1")
	c.Assert(err1, jc.ErrorIsNil)

	result2, err2 := s.startInstance(c, broker, "1/kvm/2")
	c.Assert(err2, jc.ErrorIsNil)

	callCtx := context.NewCloudCallContext()
	err := broker.StopInstances(callCtx, result0.Instance.Id())
	c.Assert(err, jc.ErrorIsNil)
	s.assertResults(c, broker, result1, result2)
	c.Assert(s.kvmContainerDir(result0), jc.DoesNotExist)
	c.Assert(s.kvmRemovedContainerDir(result0), jc.IsDirectory)

	err = broker.StopInstances(callCtx, result1.Instance.Id(), result2.Instance.Id())
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoResults(c, broker)
}

func (s *kvmBrokerSuite) TestAllRunningInstances(c *gc.C) {
	broker, brokerErr := s.newKVMBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	result0, err0 := s.startInstance(c, broker, "1/kvm/0")
	c.Assert(err0, jc.ErrorIsNil)

	result1, err1 := s.startInstance(c, broker, "1/kvm/1")
	c.Assert(err1, jc.ErrorIsNil)
	s.assertResults(c, broker, result0, result1)

	err := broker.StopInstances(context.NewCloudCallContext(), result1.Instance.Id())
	c.Assert(err, jc.ErrorIsNil)
	result2, err2 := s.startInstance(c, broker, "1/kvm/2")
	c.Assert(err2, jc.ErrorIsNil)
	s.assertResults(c, broker, result0, result2)
}

func (s *kvmBrokerSuite) assertResults(c *gc.C, broker environs.InstanceBroker, results ...*environs.StartInstanceResult) {
	assertInstancesStarted(c, broker, results...)
}

func (s *kvmBrokerSuite) assertNoResults(c *gc.C, broker environs.InstanceBroker) {
	s.assertResults(c, broker)
}

func (s *kvmBrokerSuite) kvmContainerDir(result *environs.StartInstanceResult) string {
	inst := result.Instance
	return filepath.Join(s.ContainerDir, string(inst.Id()))
}

func (s *kvmBrokerSuite) kvmRemovedContainerDir(result *environs.StartInstanceResult) string {
	inst := result.Instance
	return filepath.Join(s.RemovedDir, string(inst.Id()))
}

func (s *kvmBrokerSuite) TestStartInstancePopulatesNetworkInfo(c *gc.C) {
	broker, brokerErr := s.newKVMBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	patchResolvConf(s, c)

	result, err := s.startInstance(c, broker, "1/kvm/42")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result.NetworkInfo, gc.HasLen, 1)
	iface := result.NetworkInfo[0]
	c.Assert(iface, jc.DeepEquals, corenetwork.InterfaceInfo{
		DeviceIndex:         0,
		CIDR:                "0.1.2.0/24",
		InterfaceName:       "dummy0",
		ParentInterfaceName: "virbr0",
		MACAddress:          "aa:bb:cc:dd:ee:ff",
		Addresses:           corenetwork.ProviderAddresses{corenetwork.NewProviderAddress("0.1.2.3")},
		GatewayAddress:      corenetwork.NewProviderAddress("0.1.2.1"),
		DNSServers:          corenetwork.NewProviderAddresses("ns1.dummy", "ns2.dummy"),
		DNSSearchDomains:    []string{"dummy", "invalid"},
	})
}

func (s *kvmBrokerSuite) TestStartInstancePopulatesFallbackNetworkInfo(c *gc.C) {
	broker, brokerErr := s.newKVMBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	patchResolvConf(s, c)

	s.api.SetErrors(
		nil, // ContainerConfig succeeds
		nil, // HostChangesForContainer succeeds
		errors.NotSupportedf("container address allocation"),
	)
	_, err := s.startInstance(c, broker, "1/kvm/2")
	c.Assert(err, gc.ErrorMatches, "container address allocation not supported")
}

func (s *kvmBrokerSuite) TestStartInstanceWithCloudInitUserData(c *gc.C) {
	broker, brokerErr := s.newKVMBrokerFakeManager(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	_, err := s.startInstance(c, broker, "1/kvm/0")
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

func (s *kvmBrokerSuite) TestStartInstanceWithContainerInheritProperties(c *gc.C) {
	broker.PatchNewMachineInitReader(s, newFakeMachineInitReader)
	s.api.fakeContainerConfig.ContainerInheritProperties = "ca-certs,apt-security"

	broker, brokerErr := s.newKVMBrokerFakeManager(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	_, err := s.startInstance(c, broker, "1/kvm/0")
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
			"trusted":         []interface{}{"-----BEGIN CERTIFICATE-----\nYOUR-ORGS-TRUSTED-CA-CERT-HERE\n-----END CERTIFICATE-----\n"},
		},
	}, c)
}

type blankMachineInitReader struct {
	cloudconfig.InitReader
}

func (r *blankMachineInitReader) GetInitConfig() (map[string]interface{}, error) {
	return nil, nil
}

var newBlankMachineInitReader = func(series string) (cloudconfig.InitReader, error) {
	r, err := cloudconfig.NewMachineInitReader(series)
	return &blankMachineInitReader{r}, err
}
