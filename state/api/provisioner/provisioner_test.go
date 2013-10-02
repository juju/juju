// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/provisioner"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type provisionerSuite struct {
	testing.JujuConnSuite
	st      *api.State
	machine *state.Machine

	provisioner *provisioner.State
}

var _ = gc.Suite(&provisionerSuite{})

func (s *provisionerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	err = s.machine.SetPassword("test-password")
	c.Assert(err, gc.IsNil)
	err = s.machine.SetProvisioned("i-manager", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAsMachine(c, s.machine.Tag(), "test-password", "fake_nonce")
	c.Assert(s.st, gc.NotNil)

	// Create the provisioner API facade.
	s.provisioner = s.st.Provisioner()
	c.Assert(s.provisioner, gc.NotNil)
}

func (s *provisionerSuite) TestMachineTagAndId(c *gc.C) {
	apiMachine, err := s.provisioner.Machine("machine-42")
	c.Assert(err, gc.ErrorMatches, "machine 42 not found")
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(apiMachine, gc.IsNil)

	apiMachine, err = s.provisioner.Machine(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(apiMachine.Tag(), gc.Equals, s.machine.Tag())
	c.Assert(apiMachine.Id(), gc.Equals, s.machine.Id())
}

func (s *provisionerSuite) TestGetSetStatus(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag())
	c.Assert(err, gc.IsNil)

	status, info, err := apiMachine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusPending)
	c.Assert(info, gc.Equals, "")

	err = apiMachine.SetStatus(params.StatusStarted, "blah")
	c.Assert(err, gc.IsNil)

	status, info, err = apiMachine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStarted)
	c.Assert(info, gc.Equals, "blah")
}

func (s *provisionerSuite) TestEnsureDeadAndRemove(c *gc.C) {
	// Create a fresh machine to test the complete scenario.
	otherMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	c.Assert(otherMachine.Life(), gc.Equals, state.Alive)

	apiMachine, err := s.provisioner.Machine(otherMachine.Tag())
	c.Assert(err, gc.IsNil)

	err = apiMachine.Remove()
	c.Assert(err, gc.ErrorMatches, `cannot remove entity "machine-1": still alive`)
	err = apiMachine.EnsureDead()
	c.Assert(err, gc.IsNil)

	err = otherMachine.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(otherMachine.Life(), gc.Equals, state.Dead)

	err = apiMachine.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = otherMachine.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(otherMachine.Life(), gc.Equals, state.Dead)

	err = apiMachine.Remove()
	c.Assert(err, gc.IsNil)
	err = otherMachine.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	err = apiMachine.EnsureDead()
	c.Assert(err, gc.ErrorMatches, "machine 1 not found")
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)

	// Now try to EnsureDead machine 0 - should fail.
	apiMachine, err = s.provisioner.Machine(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	err = apiMachine.EnsureDead()
	c.Assert(err, gc.ErrorMatches, "machine 0 is required by the environment")
}

func (s *provisionerSuite) TestRefreshAndLife(c *gc.C) {
	// Create a fresh machine to test the complete scenario.
	otherMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	c.Assert(otherMachine.Life(), gc.Equals, state.Alive)

	apiMachine, err := s.provisioner.Machine(otherMachine.Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(apiMachine.Life(), gc.Equals, params.Alive)

	err = apiMachine.EnsureDead()
	c.Assert(err, gc.IsNil)
	c.Assert(apiMachine.Life(), gc.Equals, params.Alive)

	err = apiMachine.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(apiMachine.Life(), gc.Equals, params.Dead)
}

func (s *provisionerSuite) TestSetProvisionedAndInstanceId(c *gc.C) {
	// Create a fresh machine, since machine 0 is already provisioned.
	notProvisionedMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	apiMachine, err := s.provisioner.Machine(notProvisionedMachine.Tag())
	c.Assert(err, gc.IsNil)

	instanceId, err := apiMachine.InstanceId()
	c.Assert(err, jc.Satisfies, params.IsCodeNotProvisioned)
	c.Assert(err, gc.ErrorMatches, "machine 1 is not provisioned")
	c.Assert(instanceId, gc.Equals, instance.Id(""))

	hwChars := instance.MustParseHardware("cpu-cores=123", "mem=4G")
	err = apiMachine.SetProvisioned("i-will", "fake_nonce", &hwChars)
	c.Assert(err, gc.IsNil)

	instanceId, err = apiMachine.InstanceId()
	c.Assert(err, gc.IsNil)
	c.Assert(instanceId, gc.Equals, instance.Id("i-will"))

	// Try it again - should fail.
	err = apiMachine.SetProvisioned("i-wont", "fake", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set instance data for machine "1": already set`)

	// Now try to get machine 0's instance id.
	apiMachine, err = s.provisioner.Machine(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	instanceId, err = apiMachine.InstanceId()
	c.Assert(err, gc.IsNil)
	c.Assert(instanceId, gc.Equals, instance.Id("i-manager"))
}

func (s *provisionerSuite) TestSeries(c *gc.C) {
	// Create a fresh machine with different series.
	foobarMachine, err := s.State.AddMachine("foobar", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	apiMachine, err := s.provisioner.Machine(foobarMachine.Tag())
	c.Assert(err, gc.IsNil)
	series, err := apiMachine.Series()
	c.Assert(err, gc.IsNil)
	c.Assert(series, gc.Equals, "foobar")

	// Now try machine 0.
	apiMachine, err = s.provisioner.Machine(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	series, err = apiMachine.Series()
	c.Assert(err, gc.IsNil)
	c.Assert(series, gc.Equals, "quantal")
}

func (s *provisionerSuite) TestConstraints(c *gc.C) {
	// Create a fresh machine with some constraints.
	args := state.AddMachineParams{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: constraints.MustParse("cpu-cores=12", "mem=8G"),
	}
	consMachine, err := s.State.AddMachineWithConstraints(&args)
	c.Assert(err, gc.IsNil)

	apiMachine, err := s.provisioner.Machine(consMachine.Tag())
	c.Assert(err, gc.IsNil)
	cons, err := apiMachine.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, args.Constraints)

	// Now try machine 0.
	apiMachine, err = s.provisioner.Machine(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	cons, err = apiMachine.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, constraints.Value{})
}

func (s *provisionerSuite) TestWatchContainers(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag())
	c.Assert(err, gc.IsNil)

	// Add one LXC container.
	args := state.AddMachineParams{
		Series:        "quantal",
		ParentId:      s.machine.Id(),
		Jobs:          []state.MachineJob{state.JobHostUnits},
		ContainerType: instance.LXC,
	}
	container, err := s.State.AddMachineWithConstraints(&args)
	c.Assert(err, gc.IsNil)

	w, err := apiMachine.WatchContainers(instance.LXC)
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertChange(container.Id())

	// Change something other than the containers and make sure it's
	// not detected.
	err = apiMachine.SetStatus(params.StatusStarted, "not really")
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Add a KVM container and make sure it's not detected.
	args.ContainerType = instance.KVM
	container, err = s.State.AddMachineWithConstraints(&args)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Add another LXC container and make sure it's detected.
	args.ContainerType = instance.LXC
	container, err = s.State.AddMachineWithConstraints(&args)
	c.Assert(err, gc.IsNil)
	wc.AssertChange(container.Id())

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *provisionerSuite) TestWatchEnvironMachines(c *gc.C) {
	w, err := s.provisioner.WatchEnvironMachines()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertChange(s.machine.Id())

	// Add another 2 machines make sure they are detected.
	otherMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	otherMachine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	wc.AssertChange("1", "2")

	// Change the lifecycle of last machine.
	err = otherMachine.EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("2")

	// Add a container and make sure it's not detected.
	args := state.AddMachineParams{
		Series:        "quantal",
		ParentId:      s.machine.Id(),
		Jobs:          []state.MachineJob{state.JobHostUnits},
		ContainerType: instance.LXC,
	}
	_, err = s.State.AddMachineWithConstraints(&args)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *provisionerSuite) TestEnvironConfig(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)

	conf, err := s.provisioner.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(conf, gc.DeepEquals, envConfig)
}

func (s *provisionerSuite) TestWatchForEnvironConfigChanges(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)

	w, err := s.provisioner.WatchForEnvironConfigChanges()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertOneChange()

	// Change the environment configuration, check it's detected.
	attrs := envConfig.AllAttrs()
	attrs["type"] = "blah"
	newConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(newConfig)
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Change it back to the original config.
	err = s.State.SetEnvironConfig(envConfig)
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *provisionerSuite) TestStateAddresses(c *gc.C) {
	stateAddresses, err := s.State.Addresses()
	c.Assert(err, gc.IsNil)

	addresses, err := s.provisioner.StateAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.DeepEquals, stateAddresses)
}

func (s *provisionerSuite) TestAPIAddresses(c *gc.C) {
	apiInfo := s.APIInfo(c)

	addresses, err := s.provisioner.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.DeepEquals, apiInfo.Addrs)
}

func (s *provisionerSuite) TestCACert(c *gc.C) {
	caCert, err := s.provisioner.CACert()
	c.Assert(err, gc.IsNil)
	c.Assert(caCert, gc.DeepEquals, s.State.CACert())
}
