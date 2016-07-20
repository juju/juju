package reboot_test

import (
	stdtesting "testing"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apireboot "github.com/juju/juju/api/reboot"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/reboot"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type machines struct {
	machine     *state.Machine
	stateAPI    api.Connection
	rebootState apireboot.State
}

type rebootSuite struct {
	jujutesting.JujuConnSuite

	machine     *state.Machine
	stateAPI    api.Connection
	rebootState apireboot.State

	ct            *state.Machine
	ctRebootState apireboot.State

	clock clock.Clock
}

var _ = gc.Suite(&rebootSuite{})

var _ worker.NotifyWatchHandler = (*reboot.Reboot)(nil)

func (s *rebootSuite) SetUpTest(c *gc.C) {
	var err error
	template := state.MachineTemplate{
		Series: series.LatestLts(),
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	s.JujuConnSuite.SetUpTest(c)

	s.stateAPI, s.machine = s.OpenAPIAsNewMachine(c)
	s.rebootState, err = s.stateAPI.Reboot()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.rebootState, gc.NotNil)

	//Add container
	s.ct, err = s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.KVM)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.ct.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	err = s.ct.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Open api as container
	ctState := s.OpenAPIAsMachine(c, s.ct.Tag(), password, "fake_nonce")
	s.ctRebootState, err = ctState.Reboot()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.ctRebootState, gc.NotNil)

	s.clock = &fakeClock{delay: time.Millisecond}
}

func (s *rebootSuite) TearDownTest(c *gc.C) {
	s.JujuConnSuite.TearDownTest(c)
}

// NOTE: the various reboot tests use a different lock name for each test.
// This is due to the behaviour of the reboot worker. What it does is acquires
// the named process lock and never releases it. This is fine(ish) on linux as the
// garbage collector will eventually clean up the old lock which will release the
// domain socket, but on windows, the actual lock is a system level semaphore wich
// isn't cleaned up by the golang garbage collector, but instead relies on the process
// dying to release the semaphore handle.
//
// If more tests are added here, they each need their own lock name to avoid blocking
// forever on windows.

func (s *rebootSuite) TestStartStop(c *gc.C) {
	worker, err := reboot.NewReboot(s.rebootState, s.AgentConfigForTag(c, s.machine.Tag()), "test-reboot-start-stop", s.clock)
	c.Assert(err, jc.ErrorIsNil)
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

func (s *rebootSuite) TestWorkerCatchesRebootEvent(c *gc.C) {
	wrk, err := reboot.NewReboot(s.rebootState, s.AgentConfigForTag(c, s.machine.Tag()), "test-reboot-event", s.clock)
	c.Assert(err, jc.ErrorIsNil)
	err = s.rebootState.RequestReboot()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wrk.Wait(), gc.Equals, worker.ErrRebootMachine)
}

func (s *rebootSuite) TestContainerCatchesParentFlag(c *gc.C) {
	wrk, err := reboot.NewReboot(s.ctRebootState, s.AgentConfigForTag(c, s.ct.Tag()), "test-reboot-container", s.clock)
	c.Assert(err, jc.ErrorIsNil)
	err = s.rebootState.RequestReboot()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wrk.Wait(), gc.Equals, worker.ErrShutdownMachine)
}

type fakeClock struct {
	clock.Clock
	delay time.Duration
}

func (f *fakeClock) After(time.Duration) <-chan time.Time {
	return time.After(f.delay)
}
