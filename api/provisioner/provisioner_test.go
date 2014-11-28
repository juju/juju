// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(anastasia) 2014-10-08 #1378716
// Re-enable tests for PPC64/ARM64 when the fixed gccgo has been backported to trusty and the CI machines have been updated.

// +build !gccgo

package provisioner_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/provisioner"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type provisionerSuite struct {
	testing.JujuConnSuite
	*apitesting.EnvironWatcherTests
	*apitesting.APIAddresserTests

	st      *api.State
	machine *state.Machine

	provisioner *provisioner.State
}

var _ = gc.Suite(&provisionerSuite{})

func (s *provisionerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetInstanceInfo("i-manager", "fake_nonce", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAsMachine(c, s.machine.Tag(), password, "fake_nonce")
	c.Assert(s.st, gc.NotNil)
	err = s.machine.SetAddresses(network.NewAddress("0.1.2.3", network.ScopeUnknown))
	c.Assert(err, jc.ErrorIsNil)

	// Create the provisioner API facade.
	s.provisioner = s.st.Provisioner()
	c.Assert(s.provisioner, gc.NotNil)

	s.EnvironWatcherTests = apitesting.NewEnvironWatcherTests(s.provisioner, s.BackingState, apitesting.HasSecrets)
	s.APIAddresserTests = apitesting.NewAPIAddresserTests(s.provisioner, s.BackingState)
}

func (s *provisionerSuite) TestMachineTagAndId(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(names.NewMachineTag("42"))
	c.Assert(err, gc.ErrorMatches, "machine 42 not found")
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(apiMachine, gc.IsNil)

	// TODO(dfc) fix this type assertion
	apiMachine, err = s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiMachine.Tag(), gc.Equals, s.machine.Tag())
	c.Assert(apiMachine.Id(), gc.Equals, s.machine.Id())
}

func (s *provisionerSuite) TestGetSetStatus(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	status, info, err := apiMachine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, params.StatusPending)
	c.Assert(info, gc.Equals, "")

	err = apiMachine.SetStatus(params.StatusStarted, "blah", nil)
	c.Assert(err, jc.ErrorIsNil)

	status, info, err = apiMachine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, params.StatusStarted)
	c.Assert(info, gc.Equals, "blah")
	_, _, data, err := s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.HasLen, 0)
}

func (s *provisionerSuite) TestGetSetStatusWithData(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	err = apiMachine.SetStatus(params.StatusError, "blah", map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)

	status, info, err := apiMachine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, params.StatusError)
	c.Assert(info, gc.Equals, "blah")
	_, _, data, err := s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.DeepEquals, map[string]interface{}{"foo": "bar"})
}

func (s *provisionerSuite) TestMachinesWithTransientErrors(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetStatus(state.StatusError, "blah", map[string]interface{}{"transient": true})
	c.Assert(err, jc.ErrorIsNil)
	machines, info, err := s.provisioner.MachinesWithTransientErrors()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Id(), gc.Equals, "1")
	c.Assert(info, gc.HasLen, 1)
	c.Assert(info[0], gc.DeepEquals, params.StatusResult{
		Id:     "1",
		Life:   "alive",
		Status: "error",
		Info:   "blah",
		Data:   map[string]interface{}{"transient": true},
	})
}

func (s *provisionerSuite) TestEnsureDeadAndRemove(c *gc.C) {
	// Create a fresh machine to test the complete scenario.
	otherMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(otherMachine.Life(), gc.Equals, state.Alive)

	apiMachine, err := s.provisioner.Machine(otherMachine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	err = apiMachine.Remove()
	c.Assert(err, gc.ErrorMatches, `cannot remove entity "machine-1": still alive`)
	err = apiMachine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	err = otherMachine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(otherMachine.Life(), gc.Equals, state.Dead)

	err = apiMachine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = otherMachine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(otherMachine.Life(), gc.Equals, state.Dead)

	err = apiMachine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = otherMachine.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	err = apiMachine.EnsureDead()
	c.Assert(err, gc.ErrorMatches, "machine 1 not found")
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)

	// Now try to EnsureDead machine 0 - should fail.
	apiMachine, err = s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	err = apiMachine.EnsureDead()
	c.Assert(err, gc.ErrorMatches, "machine 0 is required by the environment")
}

func (s *provisionerSuite) TestRefreshAndLife(c *gc.C) {
	// Create a fresh machine to test the complete scenario.
	otherMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(otherMachine.Life(), gc.Equals, state.Alive)

	apiMachine, err := s.provisioner.Machine(otherMachine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiMachine.Life(), gc.Equals, params.Alive)

	err = apiMachine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiMachine.Life(), gc.Equals, params.Alive)

	err = apiMachine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiMachine.Life(), gc.Equals, params.Dead)
}

func (s *provisionerSuite) TestSetInstanceInfo(c *gc.C) {
	// Create a fresh machine, since machine 0 is already provisioned.
	notProvisionedMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	apiMachine, err := s.provisioner.Machine(notProvisionedMachine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	instanceId, err := apiMachine.InstanceId()
	c.Assert(err, jc.Satisfies, params.IsCodeNotProvisioned)
	c.Assert(err, gc.ErrorMatches, "machine 1 is not provisioned")
	c.Assert(instanceId, gc.Equals, instance.Id(""))

	hwChars := instance.MustParseHardware("cpu-cores=123", "mem=4G")

	_, err = s.State.Network("net1")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.State.Network("vlan42")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	ifacesMachine, err := notProvisionedMachine.NetworkInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifacesMachine, gc.HasLen, 0)

	networks := []params.Network{{
		Tag:        "network-net1",
		ProviderId: "net1",
		CIDR:       "0.1.2.0/24",
		VLANTag:    0,
	}, {
		Tag:        "network-vlan42",
		ProviderId: "vlan42",
		CIDR:       "0.2.2.0/24",
		VLANTag:    42,
	}, {
		Tag:        "network-vlan69",
		ProviderId: "vlan69",
		CIDR:       "0.3.2.0/24",
		VLANTag:    69,
	}, {
		Tag:        "network-vlan42", // duplicated; ignored
		ProviderId: "vlan42",
		CIDR:       "0.2.2.0/24",
		VLANTag:    42,
	}}
	ifaces := []params.NetworkInterface{{
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		NetworkTag:    "network-net1",
		InterfaceName: "eth0",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		NetworkTag:    "network-net1",
		InterfaceName: "eth1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		NetworkTag:    "network-vlan42",
		InterfaceName: "eth1.42",
		IsVirtual:     true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		NetworkTag:    "network-vlan69",
		InterfaceName: "eth1.69",
		IsVirtual:     true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1", // duplicated mac+net; ignored
		NetworkTag:    "network-vlan42",
		InterfaceName: "eth2",
		IsVirtual:     true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f4",
		NetworkTag:    "network-net1",
		InterfaceName: "eth1", // duplicated name+machine id; ignored
		IsVirtual:     false,
	}}

	err = apiMachine.SetInstanceInfo("i-will", "fake_nonce", &hwChars, networks, ifaces)
	c.Assert(err, jc.ErrorIsNil)

	instanceId, err = apiMachine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceId, gc.Equals, instance.Id("i-will"))

	// Try it again - should fail.
	err = apiMachine.SetInstanceInfo("i-wont", "fake", nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, `aborted instance "i-wont": cannot set instance data for machine "1": already set`)

	// Now try to get machine 0's instance id.
	apiMachine, err = s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	instanceId, err = apiMachine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceId, gc.Equals, instance.Id("i-manager"))

	// Check the networks are created.
	for i, _ := range networks {
		if i == 3 {
			// Last one was ignored, so skip it.
			break
		}
		tag, err := names.ParseNetworkTag(networks[i].Tag)
		c.Assert(err, jc.ErrorIsNil)
		networkName := tag.Id()
		network, err := s.State.Network(networkName)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(network.Name(), gc.Equals, networkName)
		c.Check(network.ProviderId(), gc.Equals, networks[i].ProviderId)
		c.Check(network.Tag().String(), gc.Equals, networks[i].Tag)
		c.Check(network.VLANTag(), gc.Equals, networks[i].VLANTag)
		c.Check(network.CIDR(), gc.Equals, networks[i].CIDR)
	}

	// And the network interfaces as well.
	ifacesMachine, err = notProvisionedMachine.NetworkInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifacesMachine, gc.HasLen, 4)
	actual := make([]params.NetworkInterface, len(ifacesMachine))
	for i, iface := range ifacesMachine {
		actual[i].InterfaceName = iface.InterfaceName()
		actual[i].NetworkTag = iface.NetworkTag().String()
		actual[i].MACAddress = iface.MACAddress()
		actual[i].IsVirtual = iface.IsVirtual()
		c.Check(iface.MachineTag(), gc.Equals, notProvisionedMachine.Tag())
		c.Check(iface.MachineId(), gc.Equals, notProvisionedMachine.Id())
	}
	c.Assert(actual, jc.SameContents, ifaces[:4]) // skip the rest as they are ignored.
}

func (s *provisionerSuite) TestSeries(c *gc.C) {
	// Create a fresh machine with different series.
	foobarMachine, err := s.State.AddMachine("foobar", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	apiMachine, err := s.provisioner.Machine(foobarMachine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	series, err := apiMachine.Series()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, gc.Equals, "foobar")

	// Now try machine 0.
	apiMachine, err = s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	series, err = apiMachine.Series()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, gc.Equals, "quantal")
}

func (s *provisionerSuite) TestDistributionGroup(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	instances, err := apiMachine.DistributionGroup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.DeepEquals, []instance.Id{"i-manager"})

	machine1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	apiMachine, err = s.provisioner.Machine(machine1.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	err = apiMachine.SetInstanceInfo("i-d", "fake", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	instances, err = apiMachine.DistributionGroup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 0) // no units assigned

	var unitNames []string
	for i := 0; i < 3; i++ {
		unit, err := wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		unitNames = append(unitNames, unit.Name())
		err = unit.AssignToMachine(machine1)
		c.Assert(err, jc.ErrorIsNil)
		instances, err := apiMachine.DistributionGroup()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(instances, gc.DeepEquals, []instance.Id{"i-d"})
	}
}

func (s *provisionerSuite) TestDistributionGroupMachineNotFound(c *gc.C) {
	stateMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	apiMachine, err := s.provisioner.Machine(stateMachine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	err = apiMachine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = apiMachine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	_, err = apiMachine.DistributionGroup()
	c.Assert(err, gc.ErrorMatches, "machine 1 not found")
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
}

func (s *provisionerSuite) TestProvisioningInfo(c *gc.C) {
	cons := constraints.MustParse("cpu-cores=12 mem=8G networks=^net3,^net4")
	template := state.MachineTemplate{
		Series:            "quantal",
		Jobs:              []state.MachineJob{state.JobHostUnits},
		Placement:         "valid",
		Constraints:       cons,
		RequestedNetworks: []string{"net1", "net2"},
	}
	machine, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)
	apiMachine, err := s.provisioner.Machine(machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	provisioningInfo, err := apiMachine.ProvisioningInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provisioningInfo.Series, gc.Equals, template.Series)
	c.Assert(provisioningInfo.Placement, gc.Equals, template.Placement)
	c.Assert(provisioningInfo.Constraints, gc.DeepEquals, template.Constraints)
	c.Assert(provisioningInfo.Networks, gc.DeepEquals, template.RequestedNetworks)
}

func (s *provisionerSuite) TestProvisioningInfoMachineNotFound(c *gc.C) {
	stateMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	apiMachine, err := s.provisioner.Machine(stateMachine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	err = apiMachine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = apiMachine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	_, err = apiMachine.ProvisioningInfo()
	c.Assert(err, gc.ErrorMatches, "machine 1 not found")
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	// auth tests in apiserver
}

func (s *provisionerSuite) TestWatchContainers(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	// Add one LXC container.
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)

	w, err := apiMachine.WatchContainers(instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertChange(container.Id())

	// Change something other than the containers and make sure it's
	// not detected.
	err = apiMachine.SetStatus(params.StatusStarted, "not really", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Add a KVM container and make sure it's not detected.
	container, err = s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.KVM)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Add another LXC container and make sure it's detected.
	container, err = s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(container.Id())

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *provisionerSuite) TestWatchContainersAcceptsSupportedContainers(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	for _, ctype := range instance.ContainerTypes {
		w, err := apiMachine.WatchContainers(ctype)
		c.Assert(w, gc.NotNil)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *provisionerSuite) TestWatchContainersErrors(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	_, err = apiMachine.WatchContainers(instance.NONE)
	c.Assert(err, gc.ErrorMatches, `unsupported container type "none"`)

	_, err = apiMachine.WatchContainers("")
	c.Assert(err, gc.ErrorMatches, "container type must be specified")
}

func (s *provisionerSuite) TestWatchEnvironMachines(c *gc.C) {
	w, err := s.provisioner.WatchEnvironMachines()
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertChange(s.machine.Id())

	// Add another 2 machines make sure they are detected.
	otherMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	otherMachine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("1", "2")

	// Change the lifecycle of last machine.
	err = otherMachine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("2")

	// Add a container and make sure it's not detected.
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	_, err = s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *provisionerSuite) TestStateAddresses(c *gc.C) {
	err := s.machine.SetAddresses(network.NewAddress("0.1.2.3", network.ScopeUnknown))
	c.Assert(err, jc.ErrorIsNil)

	stateAddresses, err := s.State.Addresses()
	c.Assert(err, jc.ErrorIsNil)

	addresses, err := s.provisioner.StateAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.DeepEquals, stateAddresses)
}

func (s *provisionerSuite) TestContainerManagerConfigKVM(c *gc.C) {
	args := params.ContainerManagerConfigParams{Type: instance.KVM}
	result, err := s.provisioner.ContainerManagerConfig(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.ManagerConfig, gc.DeepEquals, map[string]string{
		container.ConfigName: "juju",
	})
}

func (s *provisionerSuite) TestContainerManagerConfigLXC(c *gc.C) {
	args := params.ContainerManagerConfigParams{Type: instance.LXC}
	st, err := state.Open(s.MongoInfo(c), mongo.DialOpts{}, state.Policy(nil))
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	tests := []struct {
		lxcUseClone          bool
		lxcUseCloneAufs      bool
		expectedUseClone     string
		expectedUseCloneAufs string
	}{{
		lxcUseClone:          true,
		expectedUseClone:     "true",
		expectedUseCloneAufs: "false",
	}, {
		lxcUseClone:          false,
		expectedUseClone:     "false",
		expectedUseCloneAufs: "false",
	}, {
		lxcUseCloneAufs:      false,
		expectedUseClone:     "false",
		expectedUseCloneAufs: "false",
	}, {
		lxcUseClone:          true,
		lxcUseCloneAufs:      true,
		expectedUseClone:     "true",
		expectedUseCloneAufs: "true",
	}}

	result, err := s.provisioner.ContainerManagerConfig(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.ManagerConfig[container.ConfigName], gc.Equals, "juju")
	c.Assert(result.ManagerConfig["use-clone"], gc.Equals, "")

	// Change lxc-clone, and ensure it gets picked up.
	for i, t := range tests {
		c.Logf("test %d: %+v", i, t)
		err = st.UpdateEnvironConfig(map[string]interface{}{
			"lxc-clone":      t.lxcUseClone,
			"lxc-clone-aufs": t.lxcUseCloneAufs,
		}, nil, nil)
		c.Assert(err, jc.ErrorIsNil)
		result, err := s.provisioner.ContainerManagerConfig(args)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.ManagerConfig[container.ConfigName], gc.Equals, "juju")
		c.Assert(result.ManagerConfig["use-clone"], gc.Equals, t.expectedUseClone)
		c.Assert(result.ManagerConfig["use-aufs"], gc.Equals, t.expectedUseCloneAufs)
	}
}

func (s *provisionerSuite) TestContainerManagerConfigPermissive(c *gc.C) {
	// ContainerManagerConfig is permissive of container types, and
	// will just return the basic type-independent configuration.
	args := params.ContainerManagerConfigParams{Type: "invalid"}
	result, err := s.provisioner.ContainerManagerConfig(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.ManagerConfig, gc.DeepEquals, map[string]string{
		container.ConfigName: "juju",
	})
}

func (s *provisionerSuite) TestContainerConfig(c *gc.C) {
	result, err := s.provisioner.ContainerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.ProviderType, gc.Equals, "dummy")
	c.Assert(result.AuthorizedKeys, gc.Equals, s.Environ.Config().AuthorizedKeys())
	c.Assert(result.SSLHostnameVerification, jc.IsTrue)
	c.Assert(result.PreferIPv6, jc.IsTrue)
}

func (s *provisionerSuite) TestSetSupportedContainers(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	err = apiMachine.SetSupportedContainers(instance.LXC, instance.KVM)
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	containers, ok := s.machine.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, []instance.ContainerType{instance.LXC, instance.KVM})
}

func (s *provisionerSuite) TestSupportsNoContainers(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	err = apiMachine.SupportsNoContainers()
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	containers, ok := s.machine.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, []instance.ContainerType{})
}

func (s *provisionerSuite) TestFindToolsNoArch(c *gc.C) {
	s.testFindTools(c, false, nil, nil)
}

func (s *provisionerSuite) TestFindToolsArch(c *gc.C) {
	s.testFindTools(c, true, nil, nil)
}

func (s *provisionerSuite) TestFindToolsAPIError(c *gc.C) {
	apiError := errors.New("everything's broken")
	s.testFindTools(c, false, apiError, nil)
}

func (s *provisionerSuite) TestFindToolsLogicError(c *gc.C) {
	logicError := errors.NotFoundf("tools")
	s.testFindTools(c, false, nil, logicError)
}

func (s *provisionerSuite) testFindTools(c *gc.C, matchArch bool, apiError, logicError error) {
	var toolsList = coretools.List{&coretools.Tools{Version: version.Current}}
	var called bool
	provisioner.PatchFacadeCall(s, s.provisioner, func(request string, args, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "FindTools")
		expected := params.FindToolsParams{
			Number:       version.Current.Number,
			Series:       version.Current.Series,
			MinorVersion: -1,
			MajorVersion: -1,
		}
		if matchArch {
			expected.Arch = version.Current.Arch
		}
		c.Assert(args, gc.Equals, expected)
		result := response.(*params.FindToolsResult)
		result.List = toolsList
		if logicError != nil {
			result.Error = common.ServerError(logicError)
		}
		return apiError
	})

	var arch *string
	if matchArch {
		arch = &version.Current.Arch
	}
	apiList, err := s.provisioner.FindTools(version.Current.Number, version.Current.Series, arch)
	c.Assert(called, jc.IsTrue)
	if apiError != nil {
		c.Assert(err, gc.Equals, apiError)
	} else if logicError != nil {
		c.Assert(err.Error(), gc.Equals, logicError.Error())
	} else {
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(apiList, jc.SameContents, toolsList)
	}
}
