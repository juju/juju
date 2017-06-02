// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(anastasia) 2014-10-08 #1378716
// Re-enable tests for PPC64/ARM64 when the fixed gccgo has been backported to trusty and the CI machines have been updated.

// +build !gccgo

package provisioner_test

import (
	"time"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apibasetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/provisioner"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/watcher/watchertest"
)

type provisionerSuite struct {
	testing.JujuConnSuite
	*apitesting.ModelWatcherTests
	*apitesting.APIAddresserTests

	st      api.Connection
	machine *state.Machine

	provisioner *provisioner.State
}

var _ = gc.Suite(&provisionerSuite{})

func (s *provisionerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetInstanceInfo("i-manager", "fake_nonce", nil, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAsMachine(c, s.machine.Tag(), password, "fake_nonce")
	c.Assert(s.st, gc.NotNil)
	err = s.machine.SetProviderAddresses(network.NewAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	// Create the provisioner API facade.
	s.provisioner = provisioner.NewState(s.st)
	c.Assert(s.provisioner, gc.NotNil)

	s.ModelWatcherTests = apitesting.NewModelWatcherTests(s.provisioner, s.BackingState)
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

	machineStatus, info, err := apiMachine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineStatus, gc.Equals, status.Pending)
	c.Assert(info, gc.Equals, "")

	err = apiMachine.SetStatus(status.Started, "blah", nil)
	c.Assert(err, jc.ErrorIsNil)

	machineStatus, info, err = apiMachine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineStatus, gc.Equals, status.Started)
	c.Assert(info, gc.Equals, "blah")
	statusInfo, err := s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Data, gc.HasLen, 0)
}

func (s *provisionerSuite) TestGetSetInstanceStatus(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	instanceStatus, info, err := apiMachine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceStatus, gc.Equals, status.Pending)
	c.Assert(info, gc.Equals, "")
	err = apiMachine.SetInstanceStatus(status.Running, "blah", nil)
	c.Assert(err, jc.ErrorIsNil)
	instanceStatus, info, err = apiMachine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceStatus, gc.Equals, status.Running)
	c.Assert(info, gc.Equals, "blah")
	statusInfo, err := s.machine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Data, gc.HasLen, 0)
}

func (s *provisionerSuite) TestGetSetStatusWithData(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	err = apiMachine.SetStatus(status.Error, "blah", map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)

	machineStatus, info, err := apiMachine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineStatus, gc.Equals, status.Error)
	c.Assert(info, gc.Equals, "blah")
	statusInfo, err := s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Data, gc.DeepEquals, map[string]interface{}{"foo": "bar"})
}

func (s *provisionerSuite) TestMachinesWithTransientErrors(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "blah",
		Data:    map[string]interface{}{"transient": true},
		Since:   &now,
	}
	err = machine.SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	machines, info, err := s.provisioner.MachinesWithTransientErrors()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Id(), gc.Equals, "1")
	c.Assert(info, gc.HasLen, 1)
	c.Assert(info[0], gc.DeepEquals, params.StatusResult{
		Id:     "1",
		Life:   "alive",
		Status: "provisioning error",
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
	c.Assert(err, gc.ErrorMatches, "machine 0 is required by the model")
}

func (s *provisionerSuite) TestMarkForRemoval(c *gc.C) {
	machine, err := s.State.AddMachine("xenial", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	apiMachine, err := s.provisioner.Machine(machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	err = apiMachine.MarkForRemoval()
	c.Assert(err, gc.ErrorMatches, "cannot remove machine 1: machine is not dead")

	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	err = apiMachine.MarkForRemoval()
	c.Assert(err, jc.ErrorIsNil)

	removals, err := s.State.AllMachineRemovals()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(removals, jc.SameContents, []string{"1"})
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
	pm := poolmanager.New(state.NewStateSettings(s.State), provider.CommonStorageProviders())
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)

	// Create a fresh machine, since machine 0 is already provisioned.
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Volumes: []state.MachineVolumeParams{{
			Volume: state.VolumeParams{
				Pool: "loop-pool",
				Size: 123,
			}},
		},
	}
	notProvisionedMachine, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)

	apiMachine, err := s.provisioner.Machine(notProvisionedMachine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	instanceId, err := apiMachine.InstanceId()
	c.Assert(err, jc.Satisfies, params.IsCodeNotProvisioned)
	c.Assert(err, gc.ErrorMatches, "machine 1 not provisioned")
	c.Assert(instanceId, gc.Equals, instance.Id(""))

	hwChars := instance.MustParseHardware("cores=123", "mem=4G")

	volumes := []params.Volume{{
		VolumeTag: "volume-1-0",
		Info: params.VolumeInfo{
			VolumeId: "vol-123",
			Size:     124,
		},
	}}
	volumeAttachments := map[string]params.VolumeAttachmentInfo{
		"volume-1-0": {
			DeviceName: "xvdf1",
		},
	}

	err = apiMachine.SetInstanceInfo(
		"i-will", "fake_nonce", &hwChars, nil, volumes, volumeAttachments,
	)
	c.Assert(err, jc.ErrorIsNil)

	instanceId, err = apiMachine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceId, gc.Equals, instance.Id("i-will"))

	// Try it again - should fail.
	err = apiMachine.SetInstanceInfo("i-wont", "fake", nil, nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, `cannot record provisioning info for "i-wont": cannot set instance data for machine "1": already set`)

	// Now try to get machine 0's instance id.
	apiMachine, err = s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	instanceId, err = apiMachine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceId, gc.Equals, instance.Id("i-manager"))

	// Now check volumes and volume attachments.
	volume, err := s.State.Volume(names.NewVolumeTag("1/0"))
	c.Assert(err, jc.ErrorIsNil)
	volumeInfo, err := volume.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeInfo, gc.Equals, state.VolumeInfo{
		VolumeId: "vol-123",
		Pool:     "loop-pool",
		Size:     124,
	})
	stateVolumeAttachments, err := s.State.MachineVolumeAttachments(names.NewMachineTag("1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateVolumeAttachments, gc.HasLen, 1)
	volumeAttachmentInfo, err := stateVolumeAttachments[0].Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachmentInfo, gc.Equals, state.VolumeAttachmentInfo{
		DeviceName: "xvdf1",
	})
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

	err = apiMachine.SetInstanceInfo("i-d", "fake", nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	instances, err = apiMachine.DistributionGroup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 0) // no units assigned

	var unitNames []string
	for i := 0; i < 3; i++ {
		unit, err := wordpress.AddUnit(state.AddUnitParams{})
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
	// Add a couple of spaces.
	_, err := s.State.AddSpace("space1", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("space2", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	// Add 2 subnets into each space.
	// Each subnet is in a matching zone (e.g "subnet-#" in "zone#").
	testing.AddSubnetsWithTemplate(c, s.State, 4, state.SubnetInfo{
		CIDR:             "10.{{.}}.0.0/16",
		ProviderId:       "subnet-{{.}}",
		AvailabilityZone: "zone{{.}}",
		SpaceName:        "{{if (lt . 2)}}space1{{else}}space2{{end}}",
	})

	cons := constraints.MustParse("cores=12 mem=8G spaces=^space1,space2")
	template := state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Placement:   "valid",
		Constraints: cons,
	}
	machine, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)
	apiMachine, err := s.provisioner.Machine(machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	provisioningInfo, err := apiMachine.ProvisioningInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provisioningInfo.Series, gc.Equals, template.Series)
	c.Assert(provisioningInfo.Placement, gc.Equals, template.Placement)
	c.Assert(provisioningInfo.Constraints, jc.DeepEquals, template.Constraints)
	c.Assert(provisioningInfo.SubnetsToZones, jc.DeepEquals, map[string][]string{
		"subnet-2": []string{"zone2"},
		"subnet-3": []string{"zone3"},
	})
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

	// Add one LXD container.
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)

	w, err := apiMachine.WatchContainers(instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange(container.Id())

	// Change something other than the containers and make sure it's
	// not detected.
	err = apiMachine.SetStatus(status.Started, "not really", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Add a KVM container and make sure it's not detected.
	container, err = s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.KVM)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Add another LXD container and make sure it's detected.
	container, err = s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(container.Id())
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

func (s *provisionerSuite) TestWatchModelMachines(c *gc.C) {
	w, err := s.provisioner.WatchModelMachines()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

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
	_, err = s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *provisionerSuite) TestStateAddresses(c *gc.C) {
	err := s.machine.SetProviderAddresses(network.NewAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	stateAddresses, err := s.State.Addresses()
	c.Assert(err, jc.ErrorIsNil)

	addresses, err := s.provisioner.StateAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.DeepEquals, stateAddresses)
}

func (s *provisionerSuite) getManagerConfig(c *gc.C, typ instance.ContainerType) map[string]string {
	args := params.ContainerManagerConfigParams{Type: typ}
	result, err := s.provisioner.ContainerManagerConfig(args)
	c.Assert(err, jc.ErrorIsNil)
	return result.ManagerConfig
}

func (s *provisionerSuite) TestContainerManagerConfigKVM(c *gc.C) {
	cfg := s.getManagerConfig(c, instance.KVM)
	c.Assert(cfg, jc.DeepEquals, map[string]string{
		container.ConfigModelUUID: coretesting.ModelTag.Id(),
	})
}

func (s *provisionerSuite) TestContainerManagerConfigPermissive(c *gc.C) {
	// ContainerManagerConfig is permissive of container types, and
	// will just return the basic type-independent configuration.
	cfg := s.getManagerConfig(c, "invalid")
	c.Assert(cfg, jc.DeepEquals, map[string]string{
		container.ConfigModelUUID: coretesting.ModelTag.Id(),
	})
}

func (s *provisionerSuite) TestContainerConfig(c *gc.C) {
	result, err := s.provisioner.ContainerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.ProviderType, gc.Equals, "dummy")
	c.Assert(result.AuthorizedKeys, gc.Equals, s.Environ.Config().AuthorizedKeys())
	c.Assert(result.SSLHostnameVerification, jc.IsTrue)
}

func (s *provisionerSuite) TestSetSupportedContainers(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	err = apiMachine.SetSupportedContainers(instance.LXD, instance.KVM)
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	containers, ok := s.machine.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, []instance.ContainerType{instance.LXD, instance.KVM})
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
	current := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	var toolsList = coretools.List{&coretools.Tools{Version: current}}
	var called bool
	var a string
	if matchArch {
		// if matchArch is true, this will be overwriten with the host's arch, otherwise
		// leave a blank.
		a = arch.HostArch()
	}

	provisioner.PatchFacadeCall(s, s.provisioner, func(request string, args, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "FindTools")
		expected := params.FindToolsParams{
			Number:       jujuversion.Current,
			Series:       series.MustHostSeries(),
			Arch:         a,
			MinorVersion: -1,
			MajorVersion: -1,
		}
		c.Assert(args, gc.Equals, expected)
		result := response.(*params.FindToolsResult)
		result.List = toolsList
		if logicError != nil {
			result.Error = common.ServerError(logicError)
		}
		return apiError
	})
	apiList, err := s.provisioner.FindTools(jujuversion.Current, series.MustHostSeries(), a)
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

func (s *provisionerSuite) TestHostChangesForContainer(c *gc.C) {
	// Create a machine, put it in "default" space with a single NIC. Create
	// a container that is also in the "default" space, and request the
	// HostChangesForContainer to see that it wants to bridge that NIC
	_, err := s.State.AddSpace("default", network.Id("default"), nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSubnet(state.SubnetInfo{
		CIDR:      "10.0.0.0/24",
		SpaceName: "default",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       "ens3",
			Type:       state.EthernetDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   "ens3",
			CIDRAddress:  "10.0.0.10/24",
			ConfigMethod: state.StaticAddress,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	containerTemplate := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(containerTemplate, s.machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)

	changes, reconfigureDelay, err := s.provisioner.HostChangesForContainer(container.MachineTag())
	c.Assert(err, gc.ErrorMatches, "dummy provider network config not supported.*")
	c.Skip("can't test without network support")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(changes, gc.DeepEquals, []network.DeviceToBridge{{
		BridgeName: "br-ens3",
		DeviceName: "ens3",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

var _ = gc.Suite(&prepareContainerSuite{})

type prepareContainerSuite struct {
	coretesting.BaseSuite

	*gitjujutesting.Stub
}

type prepareFacade struct {
	*gitjujutesting.Stub

	prepareResult []params.NetworkConfig
	prepareError  error
}

func (f *prepareFacade) FacadeCall(objType string, version int, id, request string, requestParams, response interface{}) error {
	// We only support PrepareContainerInterfaceInfo
	if objType != "Provisioner" {
		return errors.Errorf("bad facade name: %q", objType)
	}
	if version != 0 {
		return errors.Errorf("bad version: %d", version)
	}
	if request != "PrepareContainerInterfaceInfo" {
		return errors.Errorf("bad method name: %q", request)
	}
	entities, ok := requestParams.(params.Entities)
	if !ok {
		return errors.Errorf("unknown request type: %t", requestParams)
	}
	f.Stub.AddCall("api.PrepareContainerInterfaceInfo", entities.Entities)
	if len(entities.Entities) != 1 {
		return errors.Errorf("only support a single entity not: %v", entities.Entities)
	}
	result, ok := response.(*params.MachineNetworkConfigResults)
	if !ok {
		return errors.Errorf("invalid return type: %t", response)
	}
	result.Results = make([]params.MachineNetworkConfigResult, len(entities.Entities))
	if f.prepareError != nil {
		result.Results[0].Error = common.ServerError(f.prepareError)
	} else {
		result.Results[0].Config = f.prepareResult
	}
	return nil
}

func (s *prepareContainerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.Stub = &gitjujutesting.Stub{}
}

func (s *prepareContainerSuite) apiForPrepareContainer(config []params.NetworkConfig, err error) base.APICaller {
	facade := &prepareFacade{
		Stub:          s.Stub,
		prepareResult: config,
		prepareError:  err,
	}
	return apibasetesting.APICallerFunc(facade.FacadeCall)
}

func (s *prepareContainerSuite) TestPrepareContainerInterfaceInfoNoValues(c *gc.C) {
	apicaller := s.apiForPrepareContainer(nil, nil)
	st := provisioner.NewState(apicaller)
	networkInfo, err := st.PrepareContainerInterfaceInfo(names.NewMachineTag("machine-0/lxd/0"))
	c.Assert(err, gc.IsNil)
	c.Check(networkInfo, jc.DeepEquals, []network.InterfaceInfo{})
}

func (s *prepareContainerSuite) TestPrepareContainerInterfaceInfoSingleNIC(c *gc.C) {
	apicaller := s.apiForPrepareContainer([]params.NetworkConfig{{
		DeviceIndex:         1,
		MACAddress:          "de:ad:be:ff:11:22",
		CIDR:                "192.168.0.5/24",
		MTU:                 9000,
		ProviderId:          "prov-id",
		ProviderSubnetId:    "prov-sub-id",
		ProviderSpaceId:     "prov-space-id",
		ProviderAddressId:   "prov-address-id",
		ProviderVLANId:      "prov-vlan-id",
		VLANTag:             25,
		InterfaceName:       "eth5",
		ParentInterfaceName: "parent#br-eth5",
		InterfaceType:       "ethernet",
		Disabled:            false,
		NoAutoStart:         false,
		ConfigType:          "static",
		Address:             "192.168.0.6",
		DNSServers:          []string{"8.8.8.8"},
		DNSSearchDomains:    []string{"mydomain"},
		GatewayAddress:      "192.168.0.1",
		Routes: []params.NetworkRoute{{
			DestinationCIDR: "10.0.0.0/16",
			GatewayIP:       "192.168.0.1",
			Metric:          55,
		}},
	}}, nil)
	st := provisioner.NewState(apicaller)
	networkInfo, err := st.PrepareContainerInterfaceInfo(names.NewMachineTag("machine-0/lxd/0"))
	c.Assert(err, gc.IsNil)
	c.Check(networkInfo, jc.DeepEquals, []network.InterfaceInfo{{
		DeviceIndex:         1,
		MACAddress:          "de:ad:be:ff:11:22",
		CIDR:                "192.168.0.5/24",
		MTU:                 9000,
		ProviderId:          "prov-id",
		ProviderSubnetId:    "prov-sub-id",
		ProviderSpaceId:     "prov-space-id",
		ProviderAddressId:   "prov-address-id",
		ProviderVLANId:      "prov-vlan-id",
		VLANTag:             25,
		InterfaceName:       "eth5",
		ParentInterfaceName: "parent#br-eth5",
		InterfaceType:       "ethernet",
		Disabled:            false,
		NoAutoStart:         false,
		ConfigType:          "static",
		Address:             network.NewAddress("192.168.0.6"),
		DNSServers:          network.NewAddresses("8.8.8.8"),
		DNSSearchDomains:    []string{"mydomain"},
		GatewayAddress:      network.NewAddress("192.168.0.1"),
		Routes: []network.Route{{
			DestinationCIDR: "10.0.0.0/16",
			GatewayIP:       "192.168.0.1",
			Metric:          55,
		}},
	}})
}
