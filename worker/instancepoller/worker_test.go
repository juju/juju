// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(wallyworld) - move to instancepoller_test
package instancepoller

import (
	"fmt"
	"reflect"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	apiinstancepoller "github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
)

var _ = gc.Suite(&workerSuite{})

type workerSuite struct {
	testing.JujuConnSuite

	apiSt api.Connection
	api   *apiinstancepoller.API
}

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.apiSt, _ = s.OpenAPIAsNewMachine(c, state.JobManageModel)
	s.api = s.apiSt.InstancePoller()
}

func (*workerSuite) instId(i int) instance.Id {
	return instance.Id(fmt.Sprint(i))
}

func (*workerSuite) addressesForIndex(i int) []network.Address {
	return network.NewAddresses(fmt.Sprintf("127.0.0.%d", i))
}

func (s *workerSuite) TestWorker(c *gc.C) {
	// Most functionality is already tested in detail - we
	// just need to test that things are wired together
	// correctly.

	// TODO(redir): per fwereade these should be in the worker config.
	s.PatchValue(&ShortPoll, 10*time.Millisecond)
	s.PatchValue(&LongPoll, 10*time.Millisecond)

	machines, insts := s.setupScenario(c)
	s.State.StartSync()
	w, err := NewWorker(Config{
		Delay:   time.Millisecond * 10,
		Clock:   clock.WallClock,
		Facade:  s.api,
		Environ: s.Environ,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Assert(worker.Stop(w), gc.IsNil)
	}()

	// TODO(perrito666) make this dependent on a juju status
	checkInstanceInfo := func(index int, m machine, expectedStatus string) bool {
		isProvisioned := true
		instanceStatus, err := m.InstanceStatus()
		if params.IsCodeNotProvisioned(err) {
			isProvisioned = false
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
		providerAddresses, err := m.ProviderAddresses()
		c.Assert(err, jc.ErrorIsNil)
		// TODO(perrito666) all providers should use juju statuses instead of message.
		return reflect.DeepEqual(providerAddresses, s.addressesForIndex(index)) && (!isProvisioned || instanceStatus.Info == expectedStatus)
	}

	// Wait for the odd numbered machines in the
	// first half of the machine slice to be given their
	// addresses and status.
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if !a.HasNext() {
			c.Fatalf("timed out waiting for instance info")
		}

		if machinesSatisfy(c, machines, func(i int, m *apiinstancepoller.Machine) bool {
			if i < len(machines)/2 && i%2 == 1 {
				return checkInstanceInfo(i, m, "running")
			}
			instanceStatus, err := m.InstanceStatus()
			c.Logf("instance message is: %q", instanceStatus.Info)
			c.Assert(instanceStatus.Status, gc.Equals, status.Pending.String())
			stm, err := s.State.Machine(m.Id())
			c.Assert(err, jc.ErrorIsNil)
			return len(stm.Addresses()) == 0
		}) {
			break
		}
	}
	// Now provision the even machines in the first half and watch them get addresses.
	for i := 0; i < len(insts)/2; i += 2 {
		m, err := s.State.Machine(machines[i].Id())
		c.Assert(err, jc.ErrorIsNil)
		err = m.SetProvisioned(insts[i].Id(), "nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
		dummy.SetInstanceAddresses(insts[i], s.addressesForIndex(i))
		dummy.SetInstanceStatus(insts[i], "running")
	}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if !a.HasNext() {
			c.Fatalf("timed out waiting for machine instance info")
		}
		if machinesSatisfy(c, machines, func(i int, m *apiinstancepoller.Machine) bool {
			if i < len(machines)/2 {
				return checkInstanceInfo(i, m, "running")
			}
			// Machines in second half still have no addresses, nor status.
			instanceStatus, err := m.InstanceStatus()
			c.Assert(instanceStatus.Status, gc.Equals, status.Pending.String())
			stm, err := s.State.Machine(m.Id())
			c.Assert(err, jc.ErrorIsNil)
			return len(stm.Addresses()) == 0
		}) {
			break
		}
	}

	// Provision the remaining machines and check the address and status.
	for i := len(insts) / 2; i < len(insts); i++ {
		if i%2 == 0 {
			m, err := s.State.Machine(machines[i].Id())
			c.Assert(err, jc.ErrorIsNil)
			err = m.SetProvisioned(insts[i].Id(), "nonce", nil)
			c.Assert(err, jc.ErrorIsNil)
		}
		dummy.SetInstanceAddresses(insts[i], s.addressesForIndex(i))
		dummy.SetInstanceStatus(insts[i], "running")
	}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if !a.HasNext() {
			c.Fatalf("timed out waiting for machine instance info")
		}
		if machinesSatisfy(c, machines, func(i int, m *apiinstancepoller.Machine) bool {
			return checkInstanceInfo(i, m, "running")
		}) {
			break
		}
	}
}

// TODO(rog)
// - check that the environment observer is actually hooked up.
// - check that the environment observer is stopped.
// - check that the errors propagate correctly.

func machinesSatisfy(c *gc.C, machines []*apiinstancepoller.Machine, f func(i int, m *apiinstancepoller.Machine) bool) bool {
	for i, m := range machines {
		err := m.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		if !f(i, m) {
			return false
		}
	}
	return true
}

func (s *workerSuite) setupScenario(c *gc.C) ([]*apiinstancepoller.Machine, []instance.Instance) {
	var machines []*apiinstancepoller.Machine
	var insts []instance.Instance
	for i := 0; i < 10; i++ {
		m, err := s.State.AddMachine("series", state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		apiMachine, err := s.api.Machine(names.NewMachineTag(m.Id()))
		c.Assert(err, jc.ErrorIsNil)
		machines = append(machines, apiMachine)
		inst, _ := testing.AssertStartInstance(c, s.Environ, s.ControllerConfig.ControllerUUID(), m.Id())
		insts = append(insts, inst)
	}
	// Associate the odd-numbered machines with an instance.
	for i := 1; i < len(machines); i += 2 {
		apiMachine := machines[i]
		m, err := s.State.Machine(apiMachine.Id())
		c.Assert(err, jc.ErrorIsNil)
		err = m.SetProvisioned(insts[i].Id(), "nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
	}
	// Associate the first half of the instances with an address and status.
	for i := 0; i < len(machines)/2; i++ {
		dummy.SetInstanceAddresses(insts[i], s.addressesForIndex(i))
		dummy.SetInstanceStatus(insts[i], "running")
	}
	// Make sure the second half of the instances have no addresses.
	for i := len(machines) / 2; i < len(machines); i++ {
		dummy.SetInstanceAddresses(insts[i], nil)
	}
	return machines, insts
}
