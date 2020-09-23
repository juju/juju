// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apireboot "github.com/juju/juju/api/reboot"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machinelock"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/reboot"
)

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

func (s *rebootSuite) SetUpTest(c *gc.C) {
	var err error
	template := state.MachineTemplate{
		Series: series.DefaultSupportedLTS(),
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
	err = s.ct.SetProvisioned("foo", "", "fake_nonce", nil)
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

func (s *rebootSuite) TestStartStop(c *gc.C) {
	worker, err := reboot.NewReboot(s.rebootState, s.AgentConfigForTag(c, s.machine.Tag()), &fakemachinelock{}, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

func (s *rebootSuite) TestWorkerCatchesRebootEvent(c *gc.C) {
	wrk, err := reboot.NewReboot(s.rebootState, s.AgentConfigForTag(c, s.machine.Tag()), &fakemachinelock{}, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	err = s.rebootState.RequestReboot()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wrk.Wait(), gc.Equals, worker.ErrRebootMachine)
	// The flag is cleared.
	rFlag, err := s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rFlag, jc.IsFalse)

}

func (s *rebootSuite) TestContainerCatchesParentFlag(c *gc.C) {
	wrk, err := reboot.NewReboot(s.ctRebootState, s.AgentConfigForTag(c, s.ct.Tag()), &fakemachinelock{}, s.clock)
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

type fakemachinelock struct {
	mu sync.Mutex
}

func (f *fakemachinelock) Acquire(spec machinelock.Spec) (func(), error) {
	f.mu.Lock()
	return func() {
		f.mu.Unlock()
	}, nil
}
func (f *fakemachinelock) Report(opts ...machinelock.ReportOption) (string, error) {
	return "", nil
}
