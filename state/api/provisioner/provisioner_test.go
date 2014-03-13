// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	commontesting "launchpad.net/juju-core/state/api/common/testing"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/provisioner"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type provisionerSuite struct {
	testing.JujuConnSuite
	*commontesting.EnvironWatcherTest

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
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = s.machine.SetPassword(password)
	c.Assert(err, gc.IsNil)
	err = s.machine.SetProvisioned("i-manager", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAsMachine(c, s.machine.Tag(), password, "fake_nonce")
	c.Assert(s.st, gc.NotNil)

	// Create the provisioner API facade.
	s.provisioner = s.st.Provisioner()
	c.Assert(s.provisioner, gc.NotNil)

	s.EnvironWatcherTest = commontesting.NewEnvironWatcherTest(s.provisioner, s.State, s.BackingState, commontesting.HasSecrets)
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
	template := state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: constraints.MustParse("cpu-cores=12", "mem=8G"),
	}
	consMachine, err := s.State.AddOneMachine(template)
	c.Assert(err, gc.IsNil)

	apiMachine, err := s.provisioner.Machine(consMachine.Tag())
	c.Assert(err, gc.IsNil)
	cons, err := apiMachine.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, template.Constraints)

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
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.LXC)
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
	container, err = s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.KVM)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Add another LXC container and make sure it's detected.
	container, err = s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.LXC)
	c.Assert(err, gc.IsNil)
	wc.AssertChange(container.Id())

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *provisionerSuite) TestWatchContainersAcceptsSupportedContainers(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag())
	c.Assert(err, gc.IsNil)

	for _, ctype := range instance.ContainerTypes {
		w, err := apiMachine.WatchContainers(ctype)
		c.Assert(w, gc.NotNil)
		c.Assert(err, gc.IsNil)
	}
}

func (s *provisionerSuite) TestWatchContainersErrors(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag())
	c.Assert(err, gc.IsNil)

	_, err = apiMachine.WatchContainers(instance.NONE)
	c.Assert(err, gc.ErrorMatches, `unsupported container type "none"`)

	_, err = apiMachine.WatchContainers("")
	c.Assert(err, gc.ErrorMatches, "container type must be specified")
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
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	_, err = s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.LXC)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *provisionerSuite) TestStateAddresses(c *gc.C) {
	err := s.machine.SetAddresses([]instance.Address{
		instance.NewAddress("0.1.2.3"),
	})
	c.Assert(err, gc.IsNil)

	stateAddresses, err := s.State.Addresses()
	c.Assert(err, gc.IsNil)

	addresses, err := s.provisioner.StateAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.DeepEquals, stateAddresses)
}

func (s *provisionerSuite) TestAPIAddresses(c *gc.C) {
	err := s.machine.SetAddresses([]instance.Address{
		instance.NewAddress("0.1.2.3"),
	})
	c.Assert(err, gc.IsNil)

	apiAddresses, err := s.State.APIAddresses()
	c.Assert(err, gc.IsNil)

	addresses, err := s.provisioner.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.DeepEquals, apiAddresses)
}

func (s *provisionerSuite) TestContainerConfig(c *gc.C) {
	result, err := s.provisioner.ContainerConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(result.ProviderType, gc.Equals, "dummy")
	c.Assert(result.AuthorizedKeys, gc.Equals, coretesting.FakeAuthKeys)
	c.Assert(result.SSLHostnameVerification, jc.IsTrue)
}

func (s *provisionerSuite) TestCACert(c *gc.C) {
	caCert, err := s.provisioner.CACert()
	c.Assert(err, gc.IsNil)
	c.Assert(caCert, gc.DeepEquals, s.State.CACert())
}

func (s *provisionerSuite) TestToolsWrongMachine(c *gc.C) {
	tools, err := s.provisioner.Tools("42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(tools, gc.IsNil)
}

func (s *provisionerSuite) TestTools(c *gc.C) {
	cur := version.Current
	curTools := &tools.Tools{Version: cur, URL: ""}
	curTools.Version.Minor++
	s.machine.SetAgentVersion(cur)
	// Provisioner.Tools returns the *desired* set of tools, not the
	// currently running set. We want to be upgraded to cur.Version
	stateTools, err := s.provisioner.Tools(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(stateTools.Version, gc.Equals, cur)
	c.Assert(stateTools.URL, gc.Not(gc.Equals), "")
}

func (s *provisionerSuite) TestSetSupportedContainers(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	err = apiMachine.SetSupportedContainers(instance.LXC, instance.KVM)
	c.Assert(err, gc.IsNil)

	err = s.machine.Refresh()
	c.Assert(err, gc.IsNil)
	containers, ok := s.machine.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, []instance.ContainerType{instance.LXC, instance.KVM})
}

func (s *provisionerSuite) TestSupportsNoContainers(c *gc.C) {
	apiMachine, err := s.provisioner.Machine(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	err = apiMachine.SupportsNoContainers()
	c.Assert(err, gc.IsNil)

	err = s.machine.Refresh()
	c.Assert(err, gc.IsNil)
	containers, ok := s.machine.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(containers, gc.DeepEquals, []instance.ContainerType{})
}
