package reboot_test

import (
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/fslock"
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
	rebootState *apireboot.State
}

type rebootSuite struct {
	jujutesting.JujuConnSuite

	machine     *state.Machine
	stateAPI    api.Connection
	rebootState *apireboot.State

	ct            *state.Machine
	ctRebootState *apireboot.State

	lock       *fslock.Lock
	lockReboot *fslock.Lock
}

var _ = gc.Suite(&rebootSuite{})

var _ worker.NotifyWatchHandler = (*reboot.Reboot)(nil)

func (s *rebootSuite) SetUpTest(c *gc.C) {
	var err error
	template := state.MachineTemplate{
		Series: coretesting.FakeDefaultSeries,
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

	lock, err := fslock.NewLock(c.MkDir(), "fake")
	c.Assert(err, jc.ErrorIsNil)
	s.lock = lock
}

func (s *rebootSuite) TearDownTest(c *gc.C) {
	s.JujuConnSuite.TearDownTest(c)
}

func (s *rebootSuite) TestStartStop(c *gc.C) {
	worker, err := reboot.NewReboot(s.rebootState, s.AgentConfigForTag(c, s.machine.Tag()), s.lock)
	c.Assert(err, jc.ErrorIsNil)
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

func (s *rebootSuite) TestWorkerCatchesRebootEvent(c *gc.C) {
	wrk, err := reboot.NewReboot(s.rebootState, s.AgentConfigForTag(c, s.machine.Tag()), s.lock)
	c.Assert(err, jc.ErrorIsNil)
	err = s.rebootState.RequestReboot()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wrk.Wait(), gc.Equals, worker.ErrRebootMachine)
}

func (s *rebootSuite) TestContainerCatchesParentFlag(c *gc.C) {
	wrk, err := reboot.NewReboot(s.ctRebootState, s.AgentConfigForTag(c, s.ct.Tag()), s.lock)
	c.Assert(err, jc.ErrorIsNil)
	err = s.rebootState.RequestReboot()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wrk.Wait(), gc.Equals, worker.ErrShutdownMachine)
}

func (s *rebootSuite) TestCleanupIsDoneOnBoot(c *gc.C) {
	s.lock.Lock(reboot.RebootMessage)

	wrk, err := reboot.NewReboot(s.rebootState, s.AgentConfigForTag(c, s.machine.Tag()), s.lock)
	c.Assert(err, jc.ErrorIsNil)
	wrk.Kill()
	c.Assert(wrk.Wait(), gc.IsNil)

	c.Assert(s.lock.IsLocked(), jc.IsFalse)
}
