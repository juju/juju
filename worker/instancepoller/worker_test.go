// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(wallyworld) - move to instancepoller_test
package instancepoller

import (
	"fmt"
	"reflect"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
)

var _ = gc.Suite(&workerSuite{})

type workerSuite struct {
	testing.JujuConnSuite
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
	s.PatchValue(&ShortPoll, 10*time.Millisecond)
	s.PatchValue(&LongPoll, 10*time.Millisecond)
	s.PatchValue(&gatherTime, 10*time.Millisecond)
	machines, insts := s.setupScenario(c)
	s.State.StartSync()
	w := NewWorker(s.State)
	defer func() {
		c.Assert(worker.Stop(w), gc.IsNil)
	}()

	checkInstanceInfo := func(index int, m machine, expectedStatus string) bool {
		isProvisioned := true
		status, err := m.InstanceStatus()
		if state.IsNotProvisionedError(err) {
			isProvisioned = false
		} else {
			c.Assert(err, gc.IsNil)
		}
		return reflect.DeepEqual(m.Addresses(), s.addressesForIndex(index)) && (!isProvisioned || status == expectedStatus)
	}

	// Wait for the odd numbered machines in the
	// first half of the machine slice to be given their
	// addresses and status.
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if !a.HasNext() {
			c.Fatalf("timed out waiting for instance info")
		}

		if machinesSatisfy(c, machines, func(i int, m *state.Machine) bool {
			if i < len(machines)/2 && i%2 == 1 {
				return checkInstanceInfo(i, m, "running")
			}
			status, err := m.InstanceStatus()
			if i%2 == 0 {
				// Even machines not provisioned yet.
				c.Assert(err, jc.Satisfies, state.IsNotProvisionedError)
			} else {
				c.Assert(status, gc.Equals, "")
			}
			return len(m.Addresses()) == 0
		}) {
			break
		}
	}
	// Now provision the even machines in the first half and watch them get addresses.
	for i := 0; i < len(insts)/2; i += 2 {
		m := machines[i]
		err := m.SetProvisioned(insts[i].Id(), "nonce", nil)
		c.Assert(err, gc.IsNil)
		dummy.SetInstanceAddresses(insts[i], s.addressesForIndex(i))
		dummy.SetInstanceStatus(insts[i], "running")
	}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if !a.HasNext() {
			c.Fatalf("timed out waiting for machine instance info")
		}
		if machinesSatisfy(c, machines, func(i int, m *state.Machine) bool {
			if i < len(machines)/2 {
				return checkInstanceInfo(i, m, "running")
			}
			// Machines in second half still have no addresses, nor status.
			status, err := m.InstanceStatus()
			if i%2 == 0 {
				// Even machines not provisioned yet.
				c.Assert(err, jc.Satisfies, state.IsNotProvisionedError)
			} else {
				c.Assert(status, gc.Equals, "")
			}
			return len(m.Addresses()) == 0
		}) {
			break
		}
	}

	// Provision the remaining machines and check the address and status.
	for i := len(insts) / 2; i < len(insts); i++ {
		if i%2 == 0 {
			m := machines[i]
			err := m.SetProvisioned(insts[i].Id(), "nonce", nil)
			c.Assert(err, gc.IsNil)
		}
		dummy.SetInstanceAddresses(insts[i], s.addressesForIndex(i))
		dummy.SetInstanceStatus(insts[i], "running")
	}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if !a.HasNext() {
			c.Fatalf("timed out waiting for machine instance info")
		}
		if machinesSatisfy(c, machines, func(i int, m *state.Machine) bool {
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

func machinesSatisfy(c *gc.C, machines []*state.Machine, f func(i int, m *state.Machine) bool) bool {
	for i, m := range machines {
		err := m.Refresh()
		c.Assert(err, gc.IsNil)
		if !f(i, m) {
			return false
		}
	}
	return true
}

func (s *workerSuite) setupScenario(c *gc.C) ([]*state.Machine, []instance.Instance) {
	var machines []*state.Machine
	var insts []instance.Instance
	for i := 0; i < 10; i++ {
		m, err := s.State.AddMachine("series", state.JobHostUnits)
		c.Assert(err, gc.IsNil)
		machines = append(machines, m)
		inst, _ := testing.AssertStartInstance(c, s.Environ, m.Id())
		insts = append(insts, inst)
	}
	// Associate the odd-numbered machines with an instance.
	for i := 1; i < len(machines); i += 2 {
		m := machines[i]
		err := m.SetProvisioned(insts[i].Id(), "nonce", nil)
		c.Assert(err, gc.IsNil)
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
