// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/container/kvm/mock"
	kvmtesting "github.com/juju/juju/container/kvm/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	supportedversion "github.com/juju/juju/juju/version"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/provisioner"
	"github.com/juju/juju/worker/workertest"
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
	s.PatchValue(&provisioner.GetMachineCloudInitData, func(_ string) (map[string]interface{}, error) {
		return nil, nil
	})
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
	return provisioner.NewKVMBroker(s.api.PrepareHost, s.api, manager, s.agentConfig)
}

func (s *kvmBrokerSuite) newKVMBrokerFakeManager(c *gc.C) (environs.InstanceBroker, error) {
	return provisioner.NewKVMBroker(s.api.PrepareHost, s.api, s.manager, s.agentConfig)
}

func (s *kvmBrokerSuite) maintainInstance(c *gc.C, broker environs.InstanceBroker, machineId string) {
	callMaintainInstance(c, s, broker, machineId)
}

func (s *kvmBrokerSuite) TestStartInstanceWithoutNetworkChanges(c *gc.C) {
	broker, brokerErr := s.newKVMBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)
	s.PatchValue(provisioner.GetObservedNetworkConfig, func(_ common.NetworkConfigSource) ([]params.NetworkConfig, error) {
		return nil, nil
	})
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

	err := broker.StopInstances(result0.Instance.Id())
	c.Assert(err, jc.ErrorIsNil)
	s.assertResults(c, broker, result1, result2)
	c.Assert(s.kvmContainerDir(result0), jc.DoesNotExist)
	c.Assert(s.kvmRemovedContainerDir(result0), jc.IsDirectory)

	err = broker.StopInstances(result1.Instance.Id(), result2.Instance.Id())
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoResults(c, broker)
}

func (s *kvmBrokerSuite) TestAllInstances(c *gc.C) {
	broker, brokerErr := s.newKVMBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	result0, err0 := s.startInstance(c, broker, "1/kvm/0")
	c.Assert(err0, jc.ErrorIsNil)

	result1, err1 := s.startInstance(c, broker, "1/kvm/1")
	c.Assert(err1, jc.ErrorIsNil)
	s.assertResults(c, broker, result0, result1)

	err := broker.StopInstances(result1.Instance.Id())
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
	c.Assert(iface, jc.DeepEquals, network.InterfaceInfo{
		DeviceIndex:         0,
		CIDR:                "0.1.2.0/24",
		InterfaceName:       "dummy0",
		ParentInterfaceName: "virbr0",
		MACAddress:          "aa:bb:cc:dd:ee:ff",
		Address:             network.NewAddress("0.1.2.3"),
		GatewayAddress:      network.NewAddress("0.1.2.1"),
		DNSServers:          network.NewAddresses("ns1.dummy", "ns2.dummy"),
		DNSSearchDomains:    []string{"dummy", "invalid"},
	})
}

func (s *kvmBrokerSuite) TestStartInstancePopulatesFallbackNetworkInfo(c *gc.C) {
	broker, brokerErr := s.newKVMBroker(c)
	c.Assert(brokerErr, jc.ErrorIsNil)

	s.PatchValue(provisioner.GetObservedNetworkConfig, func(_ common.NetworkConfigSource) ([]params.NetworkConfig, error) {
		return nil, nil
	})
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

type kvmProvisionerSuite struct {
	CommonProvisionerSuite
	kvmSuite
	events chan mock.Event
}

var _ = gc.Suite(&kvmProvisionerSuite{})

func (s *kvmProvisionerSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping kvm tests on windows")
	}
	s.CommonProvisionerSuite.SetUpSuite(c)
	s.kvmSuite.SetUpSuite(c)
}

func (s *kvmProvisionerSuite) TearDownSuite(c *gc.C) {
	s.kvmSuite.TearDownSuite(c)
	s.CommonProvisionerSuite.TearDownSuite(c)
}

func (s *kvmProvisionerSuite) SetUpTest(c *gc.C) {
	s.CommonProvisionerSuite.SetUpTest(c)
	s.kvmSuite.SetUpTest(c)

	s.events = make(chan mock.Event, 25)
	s.ContainerFactory.AddListener(s.events)
}

func (s *kvmProvisionerSuite) nextEvent(c *gc.C) mock.Event {
	select {
	case event := <-s.events:
		return event
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no event arrived")
	}
	panic("not reachable")
}

func (s *kvmProvisionerSuite) expectStarted(c *gc.C, machine *state.Machine) string {
	s.State.StartSync()
	event := s.nextEvent(c)
	c.Assert(event.Action, gc.Equals, mock.Started)
	err := machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	s.waitInstanceId(c, machine, instance.Id(event.InstanceId))
	return event.InstanceId
}

func (s *kvmProvisionerSuite) expectStopped(c *gc.C, instId string) {
	s.State.StartSync()
	event := s.nextEvent(c)
	c.Assert(event.Action, gc.Equals, mock.Stopped)
	c.Assert(event.InstanceId, gc.Equals, instId)
}

func (s *kvmProvisionerSuite) expectNoEvents(c *gc.C) {
	select {
	case event := <-s.events:
		c.Fatalf("unexpected event %#v", event)
	case <-time.After(coretesting.ShortWait):
		return
	}
}

func (s *kvmProvisionerSuite) TearDownTest(c *gc.C) {
	close(s.events)
	s.kvmSuite.TearDownTest(c)
	s.CommonProvisionerSuite.TearDownTest(c)
}

func noopPrepareHostFunc(names.MachineTag, loggo.Logger) error {
	return nil
}

func (s *kvmProvisionerSuite) newKvmProvisioner(c *gc.C) provisioner.Provisioner {
	machineTag := names.NewMachineTag("0")
	agentConfig := s.AgentConfigForTag(c, machineTag)
	manager := &fakeContainerManager{}
	broker, brokerErr := provisioner.NewKVMBroker(noopPrepareHostFunc, s.provisioner, manager, agentConfig)
	c.Assert(brokerErr, jc.ErrorIsNil)
	toolsFinder := (*provisioner.GetToolsFinder)(s.provisioner)
	w, err := provisioner.NewContainerProvisioner(instance.KVM, s.provisioner, agentConfig, broker, toolsFinder, &mockDistributionGroupFinder{})
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *kvmProvisionerSuite) TestProvisionerStartStop(c *gc.C) {
	p := s.newKvmProvisioner(c)
	workertest.CleanKill(c, p)
}

func (s *kvmProvisionerSuite) TestDoesNotStartEnvironMachines(c *gc.C) {
	p := s.newKvmProvisioner(c)
	defer workertest.CleanKill(c, p)

	// Check that an instance is not provisioned when the machine is created.
	_, err := s.State.AddMachine(supportedversion.SupportedLts(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.expectNoEvents(c)
}

func (s *kvmProvisionerSuite) TestDoesNotHaveRetryWatcher(c *gc.C) {
	p := s.newKvmProvisioner(c)
	defer workertest.CleanKill(c, p)

	w, err := provisioner.GetRetryWatcher(p)
	c.Assert(w, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}

func (s *kvmProvisionerSuite) addContainer(c *gc.C) *state.Machine {
	template := state.MachineTemplate{
		Series: supportedversion.SupportedLts(),
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, "0", instance.KVM)
	c.Assert(err, jc.ErrorIsNil)
	return container
}

func (s *kvmProvisionerSuite) TestContainerStartedAndStopped(c *gc.C) {
	if arch.NormaliseArch(runtime.GOARCH) != arch.AMD64 {
		c.Skip("Test only enabled on amd64, see bug lp:1572145")
	}
	p := s.newKvmProvisioner(c)
	defer workertest.CleanKill(c, p)

	container := s.addContainer(c)

	// TODO(jam): 2016-12-22 recent changes to check for networking changes
	// when starting a container cause this test to start failing, because
	// the Dummy provider does not support Networking configuration.
	_, _, err := s.provisioner.HostChangesForContainer(container.MachineTag())
	c.Assert(err, gc.ErrorMatches, "dummy provider network config not supported.*")
	c.Skip("dummy provider doesn't support network config. https://pad.lv/1651974")
	instId := s.expectStarted(c, container)

	// ...and removed, along with the machine, when the machine is Dead.
	c.Assert(container.EnsureDead(), gc.IsNil)
	s.expectStopped(c, instId)
	s.waitForRemovalMark(c, container)
}

func (s *kvmProvisionerSuite) TestKVMProvisionerObservesConfigChanges(c *gc.C) {
	p := s.newKvmProvisioner(c)
	defer workertest.CleanKill(c, p)
	s.assertProvisionerObservesConfigChanges(c, p)
}

type kvmFakeBridger struct {
	brokerSuite      *kvmBrokerSuite
	provisionerSuite *kvmProvisionerSuite
}
