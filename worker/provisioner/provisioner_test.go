// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"strings"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	apiprovisioner "launchpad.net/juju-core/state/api/provisioner"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/worker/provisioner"
)

type CommonProvisionerSuite struct {
	testing.JujuConnSuite
	op  <-chan dummy.Operation
	cfg *config.Config
	//  // defaultConstraints are used when adding a machine and then later in test assertions.
	defaultConstraints constraints.Value

	st          *api.State
	provisioner *apiprovisioner.State
}

type ProvisionerSuite struct {
	CommonProvisionerSuite
}

var _ = gc.Suite(&ProvisionerSuite{})

var veryShortAttempt = utils.AttemptStrategy{
	Total: 1 * time.Second,
	Delay: 80 * time.Millisecond,
}

func (s *CommonProvisionerSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.defaultConstraints = constraints.MustParse("arch=amd64 mem=4G cpu-cores=1 root-disk=8G")
}

func (s *CommonProvisionerSuite) SetUpTest(c *gc.C) {
	// Disable the default state policy, because the
	// provisioner needs to be able to test pathological
	// scenarios where a machine exists in state with
	// invalid environment config.
	dummy.SetStatePolicy(nil)

	s.JujuConnSuite.SetUpTest(c)
	// Create the operations channel with more than enough space
	// for those tests that don't listen on it.
	op := make(chan dummy.Operation, 500)
	dummy.Listen(op)
	s.op = op

	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	s.cfg = cfg
}

func (s *CommonProvisionerSuite) APILogin(c *gc.C, machine *state.Machine) {
	if s.st != nil {
		c.Assert(s.st.Close(), gc.IsNil)
	}
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("i-fake", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAsMachine(c, machine.Tag(), password, "fake_nonce")
	c.Assert(s.st, gc.NotNil)
	c.Logf("API: login as %q successful", machine.Tag())
	s.provisioner = s.st.Provisioner()
	c.Assert(s.provisioner, gc.NotNil)
}

// breakDummyProvider changes the environment config in state in a way
// that causes the given environMethod of the dummy provider to return
// an error, which is also returned as a message to be checked.
func breakDummyProvider(c *gc.C, st *state.State, environMethod string) string {
	oldCfg, err := st.EnvironConfig()
	c.Assert(err, gc.IsNil)
	cfg, err := oldCfg.Apply(map[string]interface{}{"broken": environMethod})
	c.Assert(err, gc.IsNil)
	err = st.SetEnvironConfig(cfg, oldCfg)
	c.Assert(err, gc.IsNil)
	return fmt.Sprintf("dummy.%s is broken", environMethod)
}

// setupEnvironment adds an environment manager machine and login to the API.
func (s *CommonProvisionerSuite) setupEnvironmentManager(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Id(), gc.Equals, "0")
	err = machine.SetAddresses([]instance.Address{
		instance.NewAddress("0.1.2.3"),
	})
	c.Assert(err, gc.IsNil)
	s.APILogin(c, machine)
}

// invalidateEnvironment alters the environment configuration
// so the Settings returned from the watcher will not pass
// validation.
func (s *CommonProvisionerSuite) invalidateEnvironment(c *gc.C) {
	attrs := s.cfg.AllAttrs()
	attrs["type"] = "unknown"
	invalidCfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(invalidCfg, s.cfg)
	c.Assert(err, gc.IsNil)
}

// fixEnvironment undoes the work of invalidateEnvironment.
func (s *CommonProvisionerSuite) fixEnvironment() error {
	cfg, err := s.State.EnvironConfig()
	if err != nil {
		return err
	}
	return s.State.SetEnvironConfig(s.cfg, cfg)
}

// stopper is stoppable.
type stopper interface {
	Stop() error
}

// stop stops a stopper.
func stop(c *gc.C, s stopper) {
	c.Assert(s.Stop(), gc.IsNil)
}

func (s *CommonProvisionerSuite) startUnknownInstance(c *gc.C, id string) instance.Instance {
	instance, _ := testing.AssertStartInstance(c, s.Conn.Environ, id)
	select {
	case o := <-s.op:
		switch o := o.(type) {
		case dummy.OpStartInstance:
		default:
			c.Fatalf("unexpected operation %#v", o)
		}
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for startinstance operation")
	}
	return instance
}

func (s *CommonProvisionerSuite) checkStartInstance(c *gc.C, m *state.Machine) instance.Instance {
	return s.checkStartInstanceCustom(c, m, "pork", s.defaultConstraints)
}

func (s *CommonProvisionerSuite) checkStartInstanceCustom(c *gc.C, m *state.Machine, secret string, cons constraints.Value) (inst instance.Instance) {
	s.BackingState.StartSync()
	for {
		select {
		case o := <-s.op:
			switch o := o.(type) {
			case dummy.OpStartInstance:
				inst = o.Instance
				s.waitInstanceId(c, m, inst.Id())

				// Check the instance was started with the expected params.
				c.Assert(o.MachineId, gc.Equals, m.Id())
				nonceParts := strings.SplitN(o.MachineNonce, ":", 2)
				c.Assert(nonceParts, gc.HasLen, 2)
				c.Assert(nonceParts[0], gc.Equals, names.MachineTag("0"))
				c.Assert(nonceParts[1], jc.Satisfies, utils.IsValidUUIDString)
				c.Assert(o.Secret, gc.Equals, secret)
				c.Assert(o.Constraints, gc.DeepEquals, cons)

				// All provisioned machines in this test suite have their hardware characteristics
				// attributes set to the same values as the constraints due to the dummy environment being used.
				hc, err := m.HardwareCharacteristics()
				c.Assert(err, gc.IsNil)
				c.Assert(*hc, gc.DeepEquals, instance.HardwareCharacteristics{
					Arch:     cons.Arch,
					Mem:      cons.Mem,
					RootDisk: cons.RootDisk,
					CpuCores: cons.CpuCores,
					CpuPower: cons.CpuPower,
					Tags:     cons.Tags,
				})
				return
			default:
				c.Logf("ignoring unexpected operation %#v", o)
			}
		case <-time.After(2 * time.Second):
			c.Fatalf("provisioner did not start an instance")
			return
		}
	}
	return
}

// checkNoOperations checks that the environ was not operated upon.
func (s *CommonProvisionerSuite) checkNoOperations(c *gc.C) {
	s.BackingState.StartSync()
	select {
	case o := <-s.op:
		c.Fatalf("unexpected operation %#v", o)
	case <-time.After(coretesting.ShortWait):
		return
	}
}

// checkStopInstances checks that an instance has been stopped.
func (s *CommonProvisionerSuite) checkStopInstances(c *gc.C, instances ...instance.Instance) {
	s.checkStopSomeInstances(c, instances, nil)
}

// checkStopSomeInstances checks that instancesToStop are stopped while instancesToKeep are not.
func (s *CommonProvisionerSuite) checkStopSomeInstances(c *gc.C,
	instancesToStop []instance.Instance, instancesToKeep []instance.Instance) {

	s.BackingState.StartSync()
	instanceIdsToStop := set.NewStrings()
	for _, instance := range instancesToStop {
		instanceIdsToStop.Add(string(instance.Id()))
	}
	instanceIdsToKeep := set.NewStrings()
	for _, instance := range instancesToKeep {
		instanceIdsToKeep.Add(string(instance.Id()))
	}
	// Continue checking for stop instance calls until all the instances we
	// are waiting on to finish, actually finish, or we time out.
	for !instanceIdsToStop.IsEmpty() {
		select {
		case o := <-s.op:
			switch o := o.(type) {
			case dummy.OpStopInstances:
				for _, stoppedInstance := range o.Instances {
					instId := string(stoppedInstance.Id())
					instanceIdsToStop.Remove(instId)
					if instanceIdsToKeep.Contains(instId) {
						c.Errorf("provisioner unexpectedly stopped instance %s", instId)
					}
				}
			default:
				c.Fatalf("unexpected operation %#v", o)
				return
			}
		case <-time.After(2 * time.Second):
			c.Fatalf("provisioner did not stop an instance")
			return
		}
	}
}

func (s *CommonProvisionerSuite) waitMachine(c *gc.C, m *state.Machine, check func() bool) {
	// TODO(jam): We need to grow a new method on NotifyWatcherC
	// that calls StartSync while waiting for changes, then
	// waitMachine and waitHardwareCharacteristics can use that
	// instead
	w := m.Watch()
	defer stop(c, w)
	timeout := time.After(coretesting.LongWait)
	resync := time.After(0)
	for {
		select {
		case <-w.Changes():
			if check() {
				return
			}
		case <-resync:
			resync = time.After(coretesting.ShortWait)
			s.BackingState.StartSync()
		case <-timeout:
			c.Fatalf("machine %v wait timed out", m)
		}
	}
}

func (s *CommonProvisionerSuite) waitHardwareCharacteristics(c *gc.C, m *state.Machine, check func() bool) {
	w := m.WatchHardwareCharacteristics()
	defer stop(c, w)
	timeout := time.After(coretesting.LongWait)
	resync := time.After(0)
	for {
		select {
		case <-w.Changes():
			if check() {
				return
			}
		case <-resync:
			resync = time.After(coretesting.ShortWait)
			s.BackingState.StartSync()
		case <-timeout:
			c.Fatalf("hardware characteristics for machine %v wait timed out", m)
		}
	}
}

// waitRemoved waits for the supplied machine to be removed from state.
func (s *CommonProvisionerSuite) waitRemoved(c *gc.C, m *state.Machine) {
	s.waitMachine(c, m, func() bool {
		err := m.Refresh()
		if errors.IsNotFoundError(err) {
			return true
		}
		c.Assert(err, gc.IsNil)
		c.Logf("machine %v is still %s", m, m.Life())
		return false
	})
}

// waitInstanceId waits until the supplied machine has an instance id, then
// asserts it is as expected.
func (s *CommonProvisionerSuite) waitInstanceId(c *gc.C, m *state.Machine, expect instance.Id) {
	s.waitHardwareCharacteristics(c, m, func() bool {
		if actual, err := m.InstanceId(); err == nil {
			c.Assert(actual, gc.Equals, expect)
			return true
		} else if !state.IsNotProvisionedError(err) {
			// We don't expect any errors.
			panic(err)
		}
		c.Logf("machine %v is still unprovisioned", m)
		return false
	})
}

func (s *ProvisionerSuite) SetUpTest(c *gc.C) {
	s.CommonProvisionerSuite.SetUpTest(c)
	s.CommonProvisionerSuite.setupEnvironmentManager(c)
}

func (s *ProvisionerSuite) newEnvironProvisioner(c *gc.C) provisioner.Provisioner {
	machineTag := "machine-0"
	agentConfig := s.AgentConfigForTag(c, machineTag)
	return provisioner.NewEnvironProvisioner(s.provisioner, agentConfig)
}

func (s *ProvisionerSuite) TestProvisionerStartStop(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	c.Assert(p.Stop(), gc.IsNil)
}

func (s *ProvisionerSuite) addMachine() (*state.Machine, error) {
	return s.BackingState.AddOneMachine(state.MachineTemplate{
		Series:      config.DefaultSeries,
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: s.defaultConstraints,
	})
}

func (s *ProvisionerSuite) TestSimple(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// Check that an instance is provisioned when the machine is created...
	m, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	instance := s.checkStartInstance(c, m)

	// ...and removed, along with the machine, when the machine is Dead.
	c.Assert(m.EnsureDead(), gc.IsNil)
	s.checkStopInstances(c, instance)
	s.waitRemoved(c, m)
}

func (s *ProvisionerSuite) TestConstraints(c *gc.C) {
	// Create a machine with non-standard constraints.
	m, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	cons := constraints.MustParse("mem=8G arch=amd64 cpu-cores=2 root-disk=10G")
	err = m.SetConstraints(cons)
	c.Assert(err, gc.IsNil)

	// Start a provisioner and check those constraints are used.
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)
	s.checkStartInstanceCustom(c, m, "pork", cons)
}

func (s *ProvisionerSuite) TestProvisionerSetsErrorStatusWhenStartInstanceFailed(c *gc.C) {
	brokenMsg := breakDummyProvider(c, s.State, "StartInstance")
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// Check that an instance is not provisioned when the machine is created...
	m, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	s.checkNoOperations(c)

	t0 := time.Now()
	for time.Since(t0) < coretesting.LongWait {
		// And check the machine status is set to error.
		status, info, _, err := m.Status()
		c.Assert(err, gc.IsNil)
		if status == params.StatusPending {
			time.Sleep(coretesting.ShortWait)
			continue
		}
		c.Assert(status, gc.Equals, params.StatusError)
		c.Assert(info, gc.Equals, brokenMsg)
		break
	}

	// Unbreak the environ config.
	err = s.fixEnvironment()
	c.Assert(err, gc.IsNil)

	// Restart the PA to make sure the machine is skipped again.
	stop(c, p)
	p = s.newEnvironProvisioner(c)
	defer stop(c, p)
	s.checkNoOperations(c)
}

func (s *ProvisionerSuite) TestProvisioningDoesNotOccurForContainers(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// create a machine to host the container.
	m, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	inst := s.checkStartInstance(c, m)

	// make a container on the machine we just created
	template := state.MachineTemplate{
		Series: config.DefaultSeries,
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, m.Id(), instance.LXC)
	c.Assert(err, gc.IsNil)

	// the PA should not attempt to create it
	s.checkNoOperations(c)

	// cleanup
	c.Assert(container.EnsureDead(), gc.IsNil)
	c.Assert(container.Remove(), gc.IsNil)
	c.Assert(m.EnsureDead(), gc.IsNil)
	s.checkStopInstances(c, inst)
	s.waitRemoved(c, m)
}

func (s *ProvisionerSuite) TestProvisioningDoesNotOccurWithAnInvalidEnvironment(c *gc.C) {
	s.invalidateEnvironment(c)

	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// try to create a machine
	_, err := s.addMachine()
	c.Assert(err, gc.IsNil)

	// the PA should not create it
	s.checkNoOperations(c)
}

func (s *ProvisionerSuite) TestProvisioningOccursWithFixedEnvironment(c *gc.C) {
	s.invalidateEnvironment(c)

	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// try to create a machine
	m, err := s.addMachine()
	c.Assert(err, gc.IsNil)

	// the PA should not create it
	s.checkNoOperations(c)

	err = s.fixEnvironment()
	c.Assert(err, gc.IsNil)

	s.checkStartInstance(c, m)
}

func (s *ProvisionerSuite) TestProvisioningDoesOccurAfterInvalidEnvironmentPublished(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// place a new machine into the state
	m, err := s.addMachine()
	c.Assert(err, gc.IsNil)

	s.checkStartInstance(c, m)

	s.invalidateEnvironment(c)

	// create a second machine
	m, err = s.addMachine()
	c.Assert(err, gc.IsNil)

	// the PA should create it using the old environment
	s.checkStartInstance(c, m)
}

func (s *ProvisionerSuite) TestProvisioningDoesNotProvisionTheSameMachineAfterRestart(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// create a machine
	m, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	s.checkStartInstance(c, m)

	// restart the PA
	stop(c, p)
	p = s.newEnvironProvisioner(c)
	defer stop(c, p)

	// check that there is only one machine provisioned.
	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Check(len(machines), gc.Equals, 2)
	c.Check(machines[0].Id(), gc.Equals, "0")
	c.Check(machines[1].CheckProvisioned("fake_nonce"), jc.IsFalse)

	// the PA should not create it a second time
	s.checkNoOperations(c)
}

func (s *ProvisionerSuite) TestProvisioningStopsInstances(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// create a machine
	m0, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	i0 := s.checkStartInstance(c, m0)

	// create a second machine
	m1, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	i1 := s.checkStartInstance(c, m1)
	stop(c, p)

	// mark the first machine as dead
	c.Assert(m0.EnsureDead(), gc.IsNil)

	// remove the second machine entirely
	c.Assert(m1.EnsureDead(), gc.IsNil)
	c.Assert(m1.Remove(), gc.IsNil)

	// start a new provisioner to shut them both down
	p = s.newEnvironProvisioner(c)
	defer stop(c, p)
	s.checkStopInstances(c, i0, i1)
	s.waitRemoved(c, m0)
}

func (s *ProvisionerSuite) TestDyingMachines(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// provision a machine
	m0, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	s.checkStartInstance(c, m0)

	// stop the provisioner and make the machine dying
	stop(c, p)
	err = m0.Destroy()
	c.Assert(err, gc.IsNil)

	// add a new, dying, unprovisioned machine
	m1, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	err = m1.Destroy()
	c.Assert(err, gc.IsNil)

	// start the provisioner and wait for it to reap the useless machine
	p = s.newEnvironProvisioner(c)
	defer stop(c, p)
	s.checkNoOperations(c)
	s.waitRemoved(c, m1)

	// verify the other one's still fine
	err = m0.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(m0.Life(), gc.Equals, state.Dying)
}

func (s *ProvisionerSuite) TestProvisioningRecoversAfterInvalidEnvironmentPublished(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// place a new machine into the state
	m, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	s.checkStartInstance(c, m)

	s.invalidateEnvironment(c)
	s.BackingState.StartSync()

	// create a second machine
	m, err = s.addMachine()
	c.Assert(err, gc.IsNil)

	// the PA should create it using the old environment
	s.checkStartInstance(c, m)

	err = s.fixEnvironment()
	c.Assert(err, gc.IsNil)

	// insert our observer
	cfgObserver := make(chan *config.Config, 1)
	provisioner.SetObserver(p, cfgObserver)

	oldcfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	attrs := oldcfg.AllAttrs()
	attrs["secret"] = "beef"
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(cfg, oldcfg)

	s.BackingState.StartSync()

	// wait for the PA to load the new configuration
	select {
	case <-cfgObserver:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("PA did not action config change")
	}

	// create a third machine
	m, err = s.addMachine()
	c.Assert(err, gc.IsNil)

	// the PA should create it using the new environment
	s.checkStartInstanceCustom(c, m, "beef", s.defaultConstraints)
}

func (s *ProvisionerSuite) TestProvisioningSafeMode(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// create a machine
	m0, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	i0 := s.checkStartInstance(c, m0)

	// create a second machine
	m1, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	i1 := s.checkStartInstance(c, m1)
	stop(c, p)

	// mark the first machine as dead
	c.Assert(m0.EnsureDead(), gc.IsNil)

	// remove the second machine entirely from state
	c.Assert(m1.EnsureDead(), gc.IsNil)
	c.Assert(m1.Remove(), gc.IsNil)

	// turn on safe mode
	oldcfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	attrs := oldcfg.AllAttrs()
	attrs["provisioner-safe-mode"] = true
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(cfg, oldcfg)

	// start a new provisioner to shut down only the machine still in state.
	p = s.newEnvironProvisioner(c)
	defer stop(c, p)
	s.checkStopSomeInstances(c, []instance.Instance{i0}, []instance.Instance{i1})
	s.waitRemoved(c, m0)
}

func (s *ProvisionerSuite) TestProvisioningSafeModeChange(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// First check that safe mode is initially off.

	// create a machine
	m0, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	i0 := s.checkStartInstance(c, m0)

	// create a second machine
	m1, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	i1 := s.checkStartInstance(c, m1)

	// mark the first machine as dead
	c.Assert(m0.EnsureDead(), gc.IsNil)

	// remove the second machine entirely from state
	c.Assert(m1.EnsureDead(), gc.IsNil)
	c.Assert(m1.Remove(), gc.IsNil)

	s.checkStopInstances(c, i0, i1)
	s.waitRemoved(c, m0)

	// insert our observer
	cfgObserver := make(chan *config.Config, 1)
	provisioner.SetObserver(p, cfgObserver)

	// turn on safe mode
	oldcfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	attrs := oldcfg.AllAttrs()
	attrs["provisioner-safe-mode"] = true
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(cfg, oldcfg)

	s.BackingState.StartSync()

	// wait for the PA to load the new configuration
	select {
	case <-cfgObserver:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("PA did not action config change")
	}

	// Now check that the provisioner has noticed safe mode is on.

	// create a machine
	m3, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	i3 := s.checkStartInstance(c, m3)

	// create an instance out of band
	i4 := s.startUnknownInstance(c, "999")

	// mark the machine as dead
	c.Assert(m3.EnsureDead(), gc.IsNil)

	// check the machine's instance is stopped, and the other isn't
	s.checkStopSomeInstances(c, []instance.Instance{i3}, []instance.Instance{i4})
	s.waitRemoved(c, m3)
}

func (s *ProvisionerSuite) newProvisionerTask(c *gc.C, safeMode bool) provisioner.ProvisionerTask {
	env := s.APIConn.Environ
	watcher, err := s.provisioner.WatchEnvironMachines()
	c.Assert(err, gc.IsNil)
	auth, err := environs.NewAPIAuthenticator(s.provisioner)
	c.Assert(err, gc.IsNil)
	return provisioner.NewProvisionerTask("machine-0", safeMode, s.provisioner, watcher, env, auth)
}

func (s *ProvisionerSuite) TestTurningOffSafeModeReapsUnknownInstances(c *gc.C) {
	task := s.newProvisionerTask(c, true)
	defer stop(c, task)

	// Initially create a machine, and an unknown instance, with safe mode on.
	m0, err := s.addMachine()
	c.Assert(err, gc.IsNil)
	i0 := s.checkStartInstance(c, m0)
	i1 := s.startUnknownInstance(c, "999")

	// mark the first machine as dead
	c.Assert(m0.EnsureDead(), gc.IsNil)

	// with safe mode on, only one of the machines is stopped.
	s.checkStopSomeInstances(c, []instance.Instance{i0}, []instance.Instance{i1})
	s.waitRemoved(c, m0)

	// turn off safe mode and check that the other machine is now stopped also.
	task.SetSafeMode(false)
	s.checkStopInstances(c, i1)
}
