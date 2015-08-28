// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	apiserverprovisioner "github.com/juju/juju/apiserver/provisioner"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/storage/provider/registry"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/provisioner"
)

type CommonProvisionerSuite struct {
	testing.JujuConnSuite
	op  <-chan dummy.Operation
	cfg *config.Config
	// defaultConstraints are used when adding a machine and then later in test assertions.
	defaultConstraints constraints.Value

	st          api.Connection
	provisioner *apiprovisioner.State
}

func (s *CommonProvisionerSuite) assertProvisionerObservesConfigChanges(c *gc.C, p provisioner.Provisioner) {
	// Inject our observer into the provisioner
	cfgObserver := make(chan *config.Config, 1)
	provisioner.SetObserver(p, cfgObserver)

	// Switch to reaping on All machines.
	attrs := map[string]interface{}{
		config.ProvisionerHarvestModeKey: config.HarvestAll.String(),
	}
	err := s.State.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.BackingState.StartSync()

	// Wait for the PA to load the new configuration. We wait for the change we expect
	// like this because sometimes we pick up the initial harvest config (destroyed)
	// rather than the one we change to (all).
	received := []string{}
	for {
		select {
		case newCfg := <-cfgObserver:
			if newCfg.ProvisionerHarvestMode().String() == config.HarvestAll.String() {
				return
			}
			received = append(received, newCfg.ProvisionerHarvestMode().String())
		case <-time.After(coretesting.LongWait):
			if len(received) == 0 {
				c.Fatalf("PA did not action config change")
			} else {
				c.Fatalf("timed out waiting for config to change to '%s', received %+v",
					config.HarvestAll.String(), received)
			}
		}
	}
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
	c.Assert(err, jc.ErrorIsNil)
	s.cfg = cfg

	// Create a machine for the dummy bootstrap instance,
	// so the provisioner doesn't destroy it.
	insts, err := s.Environ.Instances([]instance.Id{dummy.BootstrapInstanceId})
	c.Assert(err, jc.ErrorIsNil)
	addrs, err := insts[0].Addresses()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Addresses:  addrs,
		Series:     "quantal",
		Nonce:      agent.BootstrapNonce,
		InstanceId: dummy.BootstrapInstanceId,
		Jobs:       []state.MachineJob{state.JobManageEnviron},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, "0")

	err = machine.SetAgentVersion(version.Current)
	c.Assert(err, jc.ErrorIsNil)

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	s.st = s.OpenAPIAsMachine(c, machine.Tag(), password, agent.BootstrapNonce)
	c.Assert(s.st, gc.NotNil)
	c.Logf("API: login as %q successful", machine.Tag())
	s.provisioner = s.st.Provisioner()
	c.Assert(s.provisioner, gc.NotNil)
}

// breakDummyProvider changes the environment config in state in a way
// that causes the given environMethod of the dummy provider to return
// an error, which is also returned as a message to be checked.
func breakDummyProvider(c *gc.C, st *state.State, environMethod string) string {
	attrs := map[string]interface{}{"broken": environMethod}
	err := st.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	return fmt.Sprintf("dummy.%s is broken", environMethod)
}

// invalidateEnvironment alters the environment configuration
// so the Settings returned from the watcher will not pass
// validation.
func (s *CommonProvisionerSuite) invalidateEnvironment(c *gc.C) {
	st, err := state.Open(s.State.EnvironTag(), s.MongoInfo(c), mongo.DefaultDialOpts(), state.Policy(nil))
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	attrs := map[string]interface{}{"type": "unknown"}
	err = st.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

// fixEnvironment undoes the work of invalidateEnvironment.
func (s *CommonProvisionerSuite) fixEnvironment(c *gc.C) error {
	st, err := state.Open(s.State.EnvironTag(), s.MongoInfo(c), mongo.DefaultDialOpts(), state.Policy(nil))
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	attrs := map[string]interface{}{"type": s.cfg.AllAttrs()["type"]}
	return st.UpdateEnvironConfig(attrs, nil, nil)
}

// stopper is stoppable.
type stopper interface {
	Stop() error
}

// stop stops a stopper.
func stop(c *gc.C, s stopper) {
	c.Assert(s.Stop(), jc.ErrorIsNil)
}

func (s *CommonProvisionerSuite) startUnknownInstance(c *gc.C, id string) instance.Instance {
	instance, _ := testing.AssertStartInstance(c, s.Environ, id)
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
	return s.checkStartInstanceCustom(c, m, "pork", s.defaultConstraints, nil, nil, nil, nil, true, nil, true)
}

func (s *CommonProvisionerSuite) checkStartInstanceNoSecureConnection(c *gc.C, m *state.Machine) instance.Instance {
	return s.checkStartInstanceCustom(c, m, "pork", s.defaultConstraints, nil, nil, nil, nil, false, nil, true)
}

func (s *CommonProvisionerSuite) checkStartInstanceCustom(
	c *gc.C, m *state.Machine,
	secret string, cons constraints.Value,
	networks []string, networkInfo []network.InterfaceInfo,
	subnetsToZones map[network.Id][]string,
	volumes []storage.Volume,
	secureServerConnection bool,
	checkPossibleTools coretools.List,
	waitInstanceId bool,
) (
	inst instance.Instance,
) {
	s.BackingState.StartSync()
	for {
		select {
		case o := <-s.op:
			switch o := o.(type) {
			case dummy.OpStartInstance:
				inst = o.Instance
				if waitInstanceId {
					s.waitInstanceId(c, m, inst.Id())
				}

				// Check the instance was started with the expected params.
				c.Assert(o.MachineId, gc.Equals, m.Id())
				nonceParts := strings.SplitN(o.MachineNonce, ":", 2)
				c.Assert(nonceParts, gc.HasLen, 2)
				c.Assert(nonceParts[0], gc.Equals, names.NewMachineTag("0").String())
				c.Assert(nonceParts[1], jc.Satisfies, utils.IsValidUUIDString)
				c.Assert(o.Secret, gc.Equals, secret)
				c.Assert(o.SubnetsToZones, jc.DeepEquals, subnetsToZones)
				c.Assert(o.Networks, jc.DeepEquals, networks)
				c.Assert(o.NetworkInfo, jc.DeepEquals, networkInfo)
				c.Assert(o.Volumes, jc.DeepEquals, volumes)
				c.Assert(o.AgentEnvironment["SECURE_STATESERVER_CONNECTION"], gc.Equals, strconv.FormatBool(secureServerConnection))

				var jobs []multiwatcher.MachineJob
				for _, job := range m.Jobs() {
					jobs = append(jobs, job.ToParams())
				}
				c.Assert(o.Jobs, jc.SameContents, jobs)

				if checkPossibleTools != nil {
					for _, t := range o.PossibleTools {
						url := fmt.Sprintf("https://%s/environment/%s/tools/%s",
							s.st.Addr(), coretesting.EnvironmentTag.Id(), t.Version)
						c.Check(t.URL, gc.Equals, url)
						t.URL = ""
					}
					for _, t := range checkPossibleTools {
						t.URL = ""
					}
					c.Assert(o.PossibleTools, gc.DeepEquals, checkPossibleTools)
				}

				// All provisioned machines in this test suite have
				// their hardware characteristics attributes set to
				// the same values as the constraints due to the dummy
				// environment being used.
				if !constraints.IsEmpty(&cons) {
					c.Assert(o.Constraints, gc.DeepEquals, cons)
					hc, err := m.HardwareCharacteristics()
					c.Assert(err, jc.ErrorIsNil)
					c.Assert(*hc, gc.DeepEquals, instance.HardwareCharacteristics{
						Arch:     cons.Arch,
						Mem:      cons.Mem,
						RootDisk: cons.RootDisk,
						CpuCores: cons.CpuCores,
						CpuPower: cons.CpuPower,
						Tags:     cons.Tags,
					})
				}
				return
			default:
				c.Logf("ignoring unexpected operation %#v", o)
			}
		case <-time.After(2 * time.Second):
			c.Fatalf("provisioner did not start an instance")
			return
		}
	}
}

// checkNoOperations checks that the environ was not operated upon.
func (s *CommonProvisionerSuite) checkNoOperations(c *gc.C) {
	s.BackingState.StartSync()
	select {
	case o := <-s.op:
		c.Fatalf("unexpected operation %+v", o)
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
				for _, id := range o.Ids {
					instId := string(id)
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
		if errors.IsNotFound(err) {
			return true
		}
		c.Assert(err, jc.ErrorIsNil)
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
		} else if !errors.IsNotProvisioned(err) {
			// We don't expect any errors.
			panic(err)
		}
		c.Logf("machine %v is still unprovisioned", m)
		return false
	})
}

func (s *CommonProvisionerSuite) newEnvironProvisioner(c *gc.C) provisioner.Provisioner {
	machineTag := names.NewMachineTag("0")
	agentConfig := s.AgentConfigForTag(c, machineTag)
	return provisioner.NewEnvironProvisioner(s.provisioner, agentConfig)
}

func (s *CommonProvisionerSuite) addMachine() (*state.Machine, error) {
	return s.addMachineWithConstraints(s.defaultConstraints)
}

func (s *CommonProvisionerSuite) addMachineWithConstraints(cons constraints.Value) (*state.Machine, error) {
	return s.BackingState.AddOneMachine(state.MachineTemplate{
		Series:      coretesting.FakeDefaultSeries,
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: cons,
	})
}

func (s *CommonProvisionerSuite) ensureAvailability(c *gc.C, n int) []*state.Machine {
	changes, err := s.BackingState.EnsureAvailability(n, s.defaultConstraints, coretesting.FakeDefaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
	added := make([]*state.Machine, len(changes.Added))
	for i, mid := range changes.Added {
		m, err := s.BackingState.Machine(mid)
		c.Assert(err, jc.ErrorIsNil)
		added[i] = m
	}
	return added
}

func (s *ProvisionerSuite) TestProvisionerStartStop(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	c.Assert(p.Stop(), jc.ErrorIsNil)
}

func (s *ProvisionerSuite) TestSimple(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// Check that an instance is provisioned when the machine is created...
	m, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	instance := s.checkStartInstanceNoSecureConnection(c, m)

	// ...and removed, along with the machine, when the machine is Dead.
	c.Assert(m.EnsureDead(), gc.IsNil)
	s.checkStopInstances(c, instance)
	s.waitRemoved(c, m)
}

func (s *ProvisionerSuite) TestConstraints(c *gc.C) {
	// Create a machine with non-standard constraints.
	m, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("mem=8G arch=amd64 cpu-cores=2 root-disk=10G")
	err = m.SetConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)

	// Start a provisioner and check those constraints are used.
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)
	s.checkStartInstanceCustom(c, m, "pork", cons, nil, nil, nil, nil, false, nil, true)
}

func (s *ProvisionerSuite) TestPossibleTools(c *gc.C) {

	storageDir := c.MkDir()
	s.PatchValue(&tools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)

	// Set a current version that does not match the
	// agent-version in the environ config.
	currentVersion := version.MustParseBinary("1.2.3-quantal-arm64")
	s.PatchValue(&version.Current, currentVersion)

	// Upload some plausible matches, and some that should be filtered out.
	compatibleVersion := version.MustParseBinary("1.2.3-quantal-amd64")
	ignoreVersion1 := version.MustParseBinary("1.2.4-quantal-arm64")
	ignoreVersion2 := version.MustParseBinary("1.2.3-precise-arm64")
	availableVersions := []version.Binary{
		currentVersion, compatibleVersion, ignoreVersion1, ignoreVersion2,
	}
	envtesting.AssertUploadFakeToolsVersions(c, stor, s.cfg.AgentStream(), s.cfg.AgentStream(), availableVersions...)

	// Extract the tools that we expect to actually match.
	expectedList, err := tools.FindTools(s.Environ, -1, -1, s.cfg.AgentStream(), coretools.Filter{
		Number: currentVersion.Number,
		Series: currentVersion.Series,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Create the machine and check the tools that get passed into StartInstance.
	machine, err := s.BackingState.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})
	c.Assert(err, jc.ErrorIsNil)

	provisioner := s.newEnvironProvisioner(c)
	defer stop(c, provisioner)
	s.checkStartInstanceCustom(
		c, machine, "pork", constraints.Value{},
		nil, nil, nil, nil, false, expectedList, true,
	)
}

func (s *ProvisionerSuite) TestProvisionerSetsErrorStatusWhenNoToolsAreAvailable(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// Check that an instance is not provisioned when the machine is created...
	m, err := s.BackingState.AddOneMachine(state.MachineTemplate{
		// We need a valid series that has no tools uploaded
		Series:      "raring",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: s.defaultConstraints,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.checkNoOperations(c)

	t0 := time.Now()
	for time.Since(t0) < coretesting.LongWait {
		// And check the machine status is set to error.
		statusInfo, err := m.Status()
		c.Assert(err, jc.ErrorIsNil)
		if statusInfo.Status == state.StatusPending {
			time.Sleep(coretesting.ShortWait)
			continue
		}
		c.Assert(statusInfo.Status, gc.Equals, state.StatusError)
		c.Assert(statusInfo.Message, gc.Equals, "no matching tools available")
		break
	}

	// Restart the PA to make sure the machine is skipped again.
	stop(c, p)
	p = s.newEnvironProvisioner(c)
	defer stop(c, p)
	s.checkNoOperations(c)
}

func (s *ProvisionerSuite) TestProvisionerSetsErrorStatusWhenStartInstanceFailed(c *gc.C) {
	brokenMsg := breakDummyProvider(c, s.State, "StartInstance")
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// Check that an instance is not provisioned when the machine is created...
	m, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.checkNoOperations(c)

	t0 := time.Now()
	for time.Since(t0) < coretesting.LongWait {
		// And check the machine status is set to error.
		statusInfo, err := m.Status()
		c.Assert(err, jc.ErrorIsNil)
		if statusInfo.Status == state.StatusPending {
			time.Sleep(coretesting.ShortWait)
			continue
		}
		c.Assert(statusInfo.Status, gc.Equals, state.StatusError)
		c.Assert(statusInfo.Message, gc.Equals, brokenMsg)
		break
	}

	// Unbreak the environ config.
	err = s.fixEnvironment(c)
	c.Assert(err, jc.ErrorIsNil)

	// Restart the PA to make sure the machine is skipped again.
	stop(c, p)
	p = s.newEnvironProvisioner(c)
	defer stop(c, p)
	s.checkNoOperations(c)
}

func (s *ProvisionerSuite) TestProvisionerFailedStartInstanceWithInjectedCreationError(c *gc.C) {
	// create the error injection channel
	errorInjectionChannel := make(chan error, 2)

	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// patch the dummy provider error injection channel
	cleanup := dummy.PatchTransientErrorInjectionChannel(errorInjectionChannel)
	defer cleanup()

	retryableError := instance.NewRetryableCreationError("container failed to start and was destroyed")
	destroyError := errors.New("container failed to start and failed to destroy: manual cleanup of containers needed")
	// send the error message TWICE, because the provisioner will retry only ONCE
	errorInjectionChannel <- retryableError
	errorInjectionChannel <- destroyError

	m, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.checkNoOperations(c)

	t0 := time.Now()
	for time.Since(t0) < coretesting.LongWait {
		// And check the machine status is set to error.
		statusInfo, err := m.Status()
		c.Assert(err, jc.ErrorIsNil)
		if statusInfo.Status == state.StatusPending {
			time.Sleep(coretesting.ShortWait)
			continue
		}
		c.Assert(statusInfo.Status, gc.Equals, state.StatusError)
		// check that the status matches the error message
		c.Assert(statusInfo.Message, gc.Equals, destroyError.Error())
		break
	}

}

func (s *ProvisionerSuite) TestProvisionerSucceedStartInstanceWithInjectedRetryableCreationError(c *gc.C) {
	// create the error injection channel
	errorInjectionChannel := make(chan error, 1)
	c.Assert(errorInjectionChannel, gc.NotNil)

	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// patch the dummy provider error injection channel
	cleanup := dummy.PatchTransientErrorInjectionChannel(errorInjectionChannel)
	defer cleanup()

	// send the error message once
	// - instance creation should succeed
	retryableError := instance.NewRetryableCreationError("container failed to start and was destroyed")
	errorInjectionChannel <- retryableError

	m, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.checkStartInstanceNoSecureConnection(c, m)
}

func (s *ProvisionerSuite) TestProvisionerSucceedStartInstanceWithInjectedWrappedRetryableCreationError(c *gc.C) {
	// create the error injection channel
	errorInjectionChannel := make(chan error, 1)
	c.Assert(errorInjectionChannel, gc.NotNil)

	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// patch the dummy provider error injection channel
	cleanup := dummy.PatchTransientErrorInjectionChannel(errorInjectionChannel)
	defer cleanup()

	// send the error message once
	// - instance creation should succeed
	retryableError := errors.Wrap(errors.New(""), instance.NewRetryableCreationError("container failed to start and was destroyed"))
	errorInjectionChannel <- retryableError

	m, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.checkStartInstanceNoSecureConnection(c, m)
}

func (s *ProvisionerSuite) TestProvisionerFailStartInstanceWithInjectedNonRetryableCreationError(c *gc.C) {
	// create the error injection channel
	errorInjectionChannel := make(chan error, 1)
	c.Assert(errorInjectionChannel, gc.NotNil)

	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// patch the dummy provider error injection channel
	cleanup := dummy.PatchTransientErrorInjectionChannel(errorInjectionChannel)
	defer cleanup()

	// send the error message once
	// - instance creation should succeed
	nonRetryableError := errors.New("some nonretryable error")
	errorInjectionChannel <- nonRetryableError

	m, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.checkNoOperations(c)

	t0 := time.Now()
	for time.Since(t0) < coretesting.LongWait {
		// And check the machine status is set to error.
		statusInfo, err := m.Status()
		c.Assert(err, jc.ErrorIsNil)
		if statusInfo.Status == state.StatusPending {
			time.Sleep(coretesting.ShortWait)
			continue
		}
		c.Assert(statusInfo.Status, gc.Equals, state.StatusError)
		// check that the status matches the error message
		c.Assert(statusInfo.Message, gc.Equals, nonRetryableError.Error())
		break
	}
}

func (s *ProvisionerSuite) TestProvisioningDoesNotOccurForLXC(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// create a machine to host the container.
	m, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	inst := s.checkStartInstanceNoSecureConnection(c, m)

	// make a container on the machine we just created
	template := state.MachineTemplate{
		Series: coretesting.FakeDefaultSeries,
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, m.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)

	// the PA should not attempt to create it
	s.checkNoOperations(c)

	// cleanup
	c.Assert(container.EnsureDead(), gc.IsNil)
	c.Assert(container.Remove(), gc.IsNil)
	c.Assert(m.EnsureDead(), gc.IsNil)
	s.checkStopInstances(c, inst)
	s.waitRemoved(c, m)
}

func (s *ProvisionerSuite) TestProvisioningDoesNotOccurForKVM(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// create a machine to host the container.
	m, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	inst := s.checkStartInstanceNoSecureConnection(c, m)

	// make a container on the machine we just created
	template := state.MachineTemplate{
		Series: coretesting.FakeDefaultSeries,
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, m.Id(), instance.KVM)
	c.Assert(err, jc.ErrorIsNil)

	// the PA should not attempt to create it
	s.checkNoOperations(c)

	// cleanup
	c.Assert(container.EnsureDead(), gc.IsNil)
	c.Assert(container.Remove(), gc.IsNil)
	c.Assert(m.EnsureDead(), gc.IsNil)
	s.checkStopInstances(c, inst)
	s.waitRemoved(c, m)
}

type MachineClassifySuite struct {
}

var _ = gc.Suite(&MachineClassifySuite{})

type MockMachine struct {
	life          params.Life
	status        params.Status
	id            string
	idErr         error
	ensureDeadErr error
	statusErr     error
}

func (m *MockMachine) Life() params.Life {
	return m.life
}

func (m *MockMachine) InstanceId() (instance.Id, error) {
	return instance.Id(m.id), m.idErr
}

func (m *MockMachine) EnsureDead() error {
	return m.ensureDeadErr
}

func (m *MockMachine) Status() (params.Status, string, error) {
	return m.status, "", m.statusErr
}

func (m *MockMachine) Id() string {
	return m.id
}

type machineClassificationTest struct {
	description    string
	life           params.Life
	status         params.Status
	idErr          string
	ensureDeadErr  string
	expectErrCode  string
	expectErrFmt   string
	statusErr      string
	classification provisioner.MachineClassification
}

var machineClassificationTests = []machineClassificationTest{{
	description:    "Dead machine is dead",
	life:           params.Dead,
	status:         params.StatusStarted,
	classification: provisioner.Dead,
}, {
	description:    "Dying machine can carry on dying",
	life:           params.Dying,
	status:         params.StatusStarted,
	classification: provisioner.None,
}, {
	description:    "Dying unprovisioned machine is ensured dead",
	life:           params.Dying,
	status:         params.StatusStarted,
	classification: provisioner.Dead,
	idErr:          params.CodeNotProvisioned,
}, {
	description:    "Can't load provisioned dying machine",
	life:           params.Dying,
	status:         params.StatusStarted,
	classification: provisioner.None,
	idErr:          params.CodeNotFound,
	expectErrCode:  params.CodeNotFound,
	expectErrFmt:   "failed to load dying machine id:%s.*",
}, {
	description:    "Alive machine is not provisioned - pending",
	life:           params.Alive,
	status:         params.StatusPending,
	classification: provisioner.Pending,
	idErr:          params.CodeNotProvisioned,
	expectErrFmt:   "found machine pending provisioning id:%s.*",
}, {
	description:    "Alive, pending machine not found",
	life:           params.Alive,
	status:         params.StatusPending,
	classification: provisioner.None,
	idErr:          params.CodeNotFound,
	expectErrCode:  params.CodeNotFound,
	expectErrFmt:   "failed to load machine id:%s.*",
}, {
	description:    "Cannot get unprovisioned machine status",
	life:           params.Alive,
	classification: provisioner.None,
	statusErr:      params.CodeNotFound,
	idErr:          params.CodeNotProvisioned,
}, {
	description:    "Dying machine fails to ensure dead",
	life:           params.Dying,
	status:         params.StatusStarted,
	classification: provisioner.None,
	idErr:          params.CodeNotProvisioned,
	expectErrCode:  params.CodeNotFound,
	ensureDeadErr:  params.CodeNotFound,
	expectErrFmt:   "failed to ensure machine dead id:%s.*",
}}

var machineClassificationTestsRequireMaintenance = machineClassificationTest{
	description:    "Machine needs maintaining",
	life:           params.Alive,
	status:         params.StatusStarted,
	classification: provisioner.Maintain,
}

var machineClassificationTestsNoMaintenance = machineClassificationTest{
	description:    "Machine doesn't need maintaining",
	life:           params.Alive,
	status:         params.StatusStarted,
	classification: provisioner.None,
}

func (s *MachineClassifySuite) TestMachineClassification(c *gc.C) {
	test := func(t machineClassificationTest, id string) {
		// Run a sub-test from the test table
		s2e := func(s string) error {
			// Little helper to turn a non-empty string into a useful error for "ErrorMaches"
			if s != "" {
				return &params.Error{Code: s}
			}
			return nil
		}

		c.Logf("%s: %s", id, t.description)
		machine := MockMachine{t.life, t.status, id, s2e(t.idErr), s2e(t.ensureDeadErr), s2e(t.statusErr)}
		classification, err := provisioner.ClassifyMachine(&machine)
		if err != nil {
			c.Assert(err, gc.ErrorMatches, fmt.Sprintf(t.expectErrFmt, machine.Id()))
		} else {
			c.Assert(err, gc.Equals, s2e(t.expectErrCode))
		}
		c.Assert(classification, gc.Equals, t.classification)
	}

	machineIds := []string{"0/lxc/0", "0/kvm/0", "0"}
	for _, id := range machineIds {
		tests := machineClassificationTests
		if id == "0" {
			tests = append(tests, machineClassificationTestsNoMaintenance)
		} else {
			tests = append(tests, machineClassificationTestsRequireMaintenance)
		}
		for _, t := range tests {
			test(t, id)
		}
	}
}

func (s *ProvisionerSuite) TestProvisioningMachinesWithSpacesSuccess(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer p.Stop()

	// Add the spaces used in constraints.
	_, err := s.State.AddSpace("space1", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("space2", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	// Add and provision a machine with spaces specified.
	cons := constraints.MustParse(
		s.defaultConstraints.String(), "spaces=space2,^space1",
	)
	// The dummy provider simulates 2 subnets per included space.
	expectedSubnetsToZones := map[network.Id][]string{
		"subnet-0": []string{"zone0"},
		"subnet-1": []string{"zone1"},
	}
	m, err := s.addMachineWithConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)
	inst := s.checkStartInstanceCustom(
		c, m, "pork", cons,
		nil, nil,
		expectedSubnetsToZones,
		nil, false, nil, true,
	)

	// Cleanup.
	c.Assert(m.EnsureDead(), gc.IsNil)
	s.checkStopInstances(c, inst)
	s.waitRemoved(c, m)
}

func (s *ProvisionerSuite) TestProvisioningMachinesFailsWithUnknownSpaces(c *gc.C) {
	cons := constraints.MustParse(
		s.defaultConstraints.String(), "spaces=missing,ignored,^ignored-too",
	)
	m, err := s.addMachineWithConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)

	// Start the PA.
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// Expect StartInstance to fail.
	s.checkNoOperations(c)

	// Ensure machine error status was set.
	t0 := time.Now()
	for time.Since(t0) < coretesting.LongWait {
		// And check the machine status is set to error.
		statusInfo, err := m.Status()
		c.Assert(err, jc.ErrorIsNil)
		if statusInfo.Status == state.StatusPending {
			time.Sleep(coretesting.ShortWait)
			continue
		}
		c.Assert(statusInfo.Status, gc.Equals, state.StatusError)
		c.Assert(statusInfo.Message, gc.Equals, `cannot match subnets to zones: space "missing" not found`)
		break
	}

	// Make sure the task didn't stop with an error
	died := make(chan error)
	go func() {
		died <- p.Wait()
	}()
	select {
	case <-time.After(coretesting.ShortWait):
	case err := <-died:
		c.Fatalf("provisioner task died unexpectedly with err: %v", err)
	}

	// Restart the PA to make sure the machine is not retried.
	stop(c, p)
	p = s.newEnvironProvisioner(c)
	defer stop(c, p)

	s.checkNoOperations(c)
}

func (s *CommonProvisionerSuite) addMachineWithRequestedVolumes(volumes []state.MachineVolumeParams, cons constraints.Value) (*state.Machine, error) {
	return s.BackingState.AddOneMachine(state.MachineTemplate{
		Series:      coretesting.FakeDefaultSeries,
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: cons,
		Volumes:     volumes,
	})
}

func (s *ProvisionerSuite) TestProvisioningMachinesWithRequestedVolumes(c *gc.C) {
	// Set up a persistent pool.
	registry.RegisterProvider("static", &dummystorage.StorageProvider{IsDynamic: false})
	registry.RegisterEnvironStorageProviders("dummy", "static")
	defer registry.RegisterProvider("static", nil)
	poolManager := poolmanager.New(state.NewStateSettings(s.State))
	_, err := poolManager.Create("persistent-pool", "static", map[string]interface{}{"persistent": true})
	c.Assert(err, jc.ErrorIsNil)

	p := s.newEnvironProvisioner(c)
	defer p.Stop()

	// Add and provision a machine with volumes specified.
	requestedVolumes := []state.MachineVolumeParams{{
		Volume:     state.VolumeParams{Pool: "static", Size: 1024},
		Attachment: state.VolumeAttachmentParams{},
	}, {
		Volume:     state.VolumeParams{Pool: "persistent-pool", Size: 2048},
		Attachment: state.VolumeAttachmentParams{},
	}}
	expectVolumeInfo := []storage.Volume{{
		names.NewVolumeTag("1"),
		storage.VolumeInfo{
			Size: 1024,
		},
	}, {
		names.NewVolumeTag("2"),
		storage.VolumeInfo{
			Size:       2048,
			Persistent: true,
		},
	}}
	m, err := s.addMachineWithRequestedVolumes(requestedVolumes, s.defaultConstraints)
	c.Assert(err, jc.ErrorIsNil)
	inst := s.checkStartInstanceCustom(
		c, m, "pork", s.defaultConstraints,
		nil, nil, nil,
		expectVolumeInfo, false,
		nil, true,
	)

	// Cleanup.
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
	c.Assert(err, jc.ErrorIsNil)

	// the PA should not create it
	s.checkNoOperations(c)
}

func (s *ProvisionerSuite) TestProvisioningOccursWithFixedEnvironment(c *gc.C) {
	s.invalidateEnvironment(c)

	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// try to create a machine
	m, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)

	// the PA should not create it
	s.checkNoOperations(c)

	err = s.fixEnvironment(c)
	c.Assert(err, jc.ErrorIsNil)

	s.checkStartInstanceNoSecureConnection(c, m)
}

func (s *ProvisionerSuite) TestProvisioningDoesOccurAfterInvalidEnvironmentPublished(c *gc.C) {
	s.PatchValue(provisioner.GetToolsFinder, func(*apiprovisioner.State) provisioner.ToolsFinder {
		return mockToolsFinder{}
	})
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// place a new machine into the state
	m, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)

	s.checkStartInstanceNoSecureConnection(c, m)

	s.invalidateEnvironment(c)

	// create a second machine
	m, err = s.addMachine()
	c.Assert(err, jc.ErrorIsNil)

	// the PA should create it using the old environment
	s.checkStartInstanceNoSecureConnection(c, m)
}

func (s *ProvisionerSuite) TestProvisioningDoesNotProvisionTheSameMachineAfterRestart(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// create a machine
	m, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.checkStartInstanceNoSecureConnection(c, m)

	// restart the PA
	stop(c, p)
	p = s.newEnvironProvisioner(c)
	defer stop(c, p)

	// check that there is only one machine provisioned.
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(len(machines), gc.Equals, 2)
	c.Check(machines[0].Id(), gc.Equals, "0")
	c.Check(machines[1].CheckProvisioned("fake_nonce"), jc.IsFalse)

	// the PA should not create it a second time
	s.checkNoOperations(c)
}

func (s *ProvisionerSuite) TestDyingMachines(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// provision a machine
	m0, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.checkStartInstanceNoSecureConnection(c, m0)

	// stop the provisioner and make the machine dying
	stop(c, p)
	err = m0.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// add a new, dying, unprovisioned machine
	m1, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	err = m1.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// start the provisioner and wait for it to reap the useless machine
	p = s.newEnvironProvisioner(c)
	defer stop(c, p)
	s.checkNoOperations(c)
	s.waitRemoved(c, m1)

	// verify the other one's still fine
	err = m0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.Life(), gc.Equals, state.Dying)
}

func (s *ProvisionerSuite) TestProvisioningRecoversAfterInvalidEnvironmentPublished(c *gc.C) {
	s.PatchValue(provisioner.GetToolsFinder, func(*apiprovisioner.State) provisioner.ToolsFinder {
		return mockToolsFinder{}
	})
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)

	// place a new machine into the state
	m, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.checkStartInstanceNoSecureConnection(c, m)

	s.invalidateEnvironment(c)
	s.BackingState.StartSync()

	// create a second machine
	m, err = s.addMachine()
	c.Assert(err, jc.ErrorIsNil)

	// the PA should create it using the old environment
	s.checkStartInstanceNoSecureConnection(c, m)

	err = s.fixEnvironment(c)
	c.Assert(err, jc.ErrorIsNil)

	// insert our observer
	cfgObserver := make(chan *config.Config, 1)
	provisioner.SetObserver(p, cfgObserver)

	err = s.State.UpdateEnvironConfig(map[string]interface{}{"secret": "beef"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.BackingState.StartSync()

	// wait for the PA to load the new configuration
	select {
	case <-cfgObserver:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("PA did not action config change")
	}

	// create a third machine
	m, err = s.addMachine()
	c.Assert(err, jc.ErrorIsNil)

	// the PA should create it using the new environment
	s.checkStartInstanceCustom(c, m, "beef", s.defaultConstraints, nil, nil, nil, nil, false, nil, true)
}

type mockMachineGetter struct{}

func (*mockMachineGetter) Machine(names.MachineTag) (*apiprovisioner.Machine, error) {
	return nil, fmt.Errorf("error")
}

func (*mockMachineGetter) MachinesWithTransientErrors() ([]*apiprovisioner.Machine, []params.StatusResult, error) {
	return nil, nil, fmt.Errorf("error")
}

func (s *ProvisionerSuite) TestMachineErrorsRetainInstances(c *gc.C) {
	task := s.newProvisionerTask(c, config.HarvestAll, s.Environ, s.provisioner, mockToolsFinder{})
	defer stop(c, task)

	// create a machine
	m0, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.checkStartInstance(c, m0)

	// create an instance out of band
	s.startUnknownInstance(c, "999")

	// start the provisioner and ensure it doesn't kill any instances if there are error getting machines
	task = s.newProvisionerTask(
		c,
		config.HarvestAll,
		s.Environ,
		&mockMachineGetter{},
		&mockToolsFinder{},
	)
	defer func() {
		err := task.Stop()
		c.Assert(err, gc.ErrorMatches, ".*failed to get machine.*")
	}()
	s.checkNoOperations(c)
}

func (s *ProvisionerSuite) TestEnvironProvisionerObservesConfigChanges(c *gc.C) {
	p := s.newEnvironProvisioner(c)
	defer stop(c, p)
	s.assertProvisionerObservesConfigChanges(c, p)
}

func (s *ProvisionerSuite) newProvisionerTask(
	c *gc.C,
	harvestingMethod config.HarvestMode,
	broker environs.InstanceBroker,
	machineGetter provisioner.MachineGetter,
	toolsFinder provisioner.ToolsFinder,
) provisioner.ProvisionerTask {

	machineWatcher, err := s.provisioner.WatchEnvironMachines()
	c.Assert(err, jc.ErrorIsNil)
	retryWatcher, err := s.provisioner.WatchMachineErrorRetry()
	c.Assert(err, jc.ErrorIsNil)
	auth, err := authentication.NewAPIAuthenticator(s.provisioner)
	c.Assert(err, jc.ErrorIsNil)

	return provisioner.NewProvisionerTask(
		names.NewMachineTag("0"),
		harvestingMethod,
		machineGetter,
		toolsFinder,
		machineWatcher,
		retryWatcher,
		broker,
		auth,
		imagemetadata.ReleasedStream,
		true,
	)
}

func (s *ProvisionerSuite) TestHarvestNoneReapsNothing(c *gc.C) {

	task := s.newProvisionerTask(c, config.HarvestDestroyed, s.Environ, s.provisioner, mockToolsFinder{})
	defer stop(c, task)
	task.SetHarvestMode(config.HarvestNone)

	// Create a machine and an unknown instance.
	m0, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.checkStartInstance(c, m0)
	s.startUnknownInstance(c, "999")

	// Mark the first machine as dead.
	c.Assert(m0.EnsureDead(), gc.IsNil)

	// Ensure we're doing nothing.
	s.checkNoOperations(c)
}

func (s *ProvisionerSuite) TestHarvestUnknownReapsOnlyUnknown(c *gc.C) {

	task := s.newProvisionerTask(c,
		config.HarvestDestroyed,
		s.Environ,
		s.provisioner,
		mockToolsFinder{},
	)
	defer stop(c, task)
	task.SetHarvestMode(config.HarvestUnknown)

	// Create a machine and an unknown instance.
	m0, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	i0 := s.checkStartInstance(c, m0)
	i1 := s.startUnknownInstance(c, "999")

	// Mark the first machine as dead.
	c.Assert(m0.EnsureDead(), gc.IsNil)

	// When only harvesting unknown machines, only one of the machines
	// is stopped.
	s.checkStopSomeInstances(c, []instance.Instance{i1}, []instance.Instance{i0})
	s.waitRemoved(c, m0)
}

func (s *ProvisionerSuite) TestHarvestDestroyedReapsOnlyDestroyed(c *gc.C) {

	task := s.newProvisionerTask(
		c,
		config.HarvestDestroyed,
		s.Environ,
		s.provisioner,
		mockToolsFinder{},
	)
	defer stop(c, task)

	// Create a machine and an unknown instance.
	m0, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	i0 := s.checkStartInstance(c, m0)
	i1 := s.startUnknownInstance(c, "999")

	// Mark the first machine as dead.
	c.Assert(m0.EnsureDead(), gc.IsNil)

	// When only harvesting destroyed machines, only one of the
	// machines is stopped.
	s.checkStopSomeInstances(c, []instance.Instance{i0}, []instance.Instance{i1})
	s.waitRemoved(c, m0)
}

func (s *ProvisionerSuite) TestHarvestAllReapsAllTheThings(c *gc.C) {

	task := s.newProvisionerTask(c,
		config.HarvestDestroyed,
		s.Environ,
		s.provisioner,
		mockToolsFinder{},
	)
	defer stop(c, task)
	task.SetHarvestMode(config.HarvestAll)

	// Create a machine and an unknown instance.
	m0, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	i0 := s.checkStartInstance(c, m0)
	i1 := s.startUnknownInstance(c, "999")

	// Mark the first machine as dead.
	c.Assert(m0.EnsureDead(), gc.IsNil)

	// Everything must die!
	s.checkStopSomeInstances(c, []instance.Instance{i0, i1}, []instance.Instance{})
	s.waitRemoved(c, m0)
}

func (s *ProvisionerSuite) TestProvisionerRetriesTransientErrors(c *gc.C) {
	s.PatchValue(&apiserverprovisioner.ErrorRetryWaitDelay, 5*time.Millisecond)
	e := &mockBroker{Environ: s.Environ, retryCount: make(map[string]int)}
	task := s.newProvisionerTask(c, config.HarvestAll, e, s.provisioner, mockToolsFinder{})
	defer stop(c, task)

	// Provision some machines, some will be started first time,
	// another will require retries.
	m1, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.checkStartInstance(c, m1)
	m2, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.checkStartInstance(c, m2)
	m3, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)
	m4, err := s.addMachine()
	c.Assert(err, jc.ErrorIsNil)

	// mockBroker will fail to start machine-3 several times;
	// keep setting the transient flag to retry until the
	// instance has started.
	thatsAllFolks := make(chan struct{})
	go func() {
		for {
			select {
			case <-thatsAllFolks:
				return
			case <-time.After(coretesting.ShortWait):
				err := m3.SetStatus(state.StatusError, "info", map[string]interface{}{"transient": true})
				c.Assert(err, jc.ErrorIsNil)
			}
		}
	}()
	s.checkStartInstance(c, m3)
	close(thatsAllFolks)

	// Machine 4 is never provisioned.
	statusInfo, err := m4.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusError)
	_, err = m4.InstanceId()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
}

func (s *ProvisionerSuite) TestProvisionerObservesMachineJobs(c *gc.C) {
	s.PatchValue(&apiserverprovisioner.ErrorRetryWaitDelay, 5*time.Millisecond)
	broker := &mockBroker{Environ: s.Environ, retryCount: make(map[string]int)}
	task := s.newProvisionerTask(c, config.HarvestAll, broker, s.provisioner, mockToolsFinder{})
	defer stop(c, task)

	added := s.ensureAvailability(c, 3)
	c.Assert(added, gc.HasLen, 2)
	byId := make(map[string]*state.Machine)
	for _, m := range added {
		byId[m.Id()] = m
	}
	for _, id := range broker.ids {
		s.checkStartInstance(c, byId[id])
	}
}

type mockBroker struct {
	environs.Environ
	retryCount map[string]int
	ids        []string
}

func (b *mockBroker) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	// All machines except machines 3, 4 are provisioned successfully the first time.
	// Machines 3 is provisioned after some attempts have been made.
	// Machine 4 is never provisioned.
	id := args.InstanceConfig.MachineId
	// record ids so we can call checkStartInstance in the appropriate order.
	b.ids = append(b.ids, id)
	retries := b.retryCount[id]
	if (id != "3" && id != "4") || retries > 2 {
		return b.Environ.StartInstance(args)
	} else {
		b.retryCount[id] = retries + 1
	}
	return nil, fmt.Errorf("error: some error")
}

type mockToolsFinder struct {
}

func (f mockToolsFinder) FindTools(number version.Number, series string, a *string) (coretools.List, error) {
	v, err := version.ParseBinary(fmt.Sprintf("%s-%s-%s", number, series, arch.HostArch()))
	if err != nil {
		return nil, err
	}
	if a != nil {
		v.Arch = *a
	}
	return coretools.List{&coretools.Tools{Version: v}}, nil
}
