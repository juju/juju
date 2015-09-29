// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiaddresser "github.com/juju/juju/api/addresser"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/addresser"
)

type workerSuite struct {
	testing.JujuConnSuite

	Enabled  bool
	MachineA *state.Machine
	MachineB *state.Machine

	Worker  worker.Worker
	OpsChan chan dummy.Operation

	APIConnection api.Connection
	API           *apiaddresser.API
}

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	if s.Enabled {
		s.SetFeatureFlags(feature.AddressAllocation)
	}

	// Unbreak dummy provider methods.
	s.AssertConfigParameterUpdated(c, "broken", "")

	s.APIConnection, _ = s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	s.API = s.APIConnection.Addresser()

	machineA, err := s.State.AddMachine("quantal", state.JobHostUnits)
	s.MachineA = machineA
	c.Assert(err, jc.ErrorIsNil)
	err = s.MachineA.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	// This machine will be destroyed after address creation to test the
	// handling of addresses for machines that have gone.
	machineB, err := s.State.AddMachine("quantal", state.JobHostUnits)
	s.MachineB = machineB
	c.Assert(err, jc.ErrorIsNil)

	s.createAddresses(c)
	s.State.StartSync()

	s.OpsChan = make(chan dummy.Operation, 10)
	dummy.Listen(s.OpsChan)

	// Start the Addresser worker.
	w, err := addresser.NewWorker(s.API)
	c.Assert(err, jc.ErrorIsNil)
	s.Worker = w

	s.waitForInitialDead(c)
}

func (s *workerSuite) TearDownTest(c *gc.C) {
	c.Assert(worker.Stop(s.Worker), jc.ErrorIsNil)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *workerSuite) createAddresses(c *gc.C) {
	addresses := []string{
		"0.1.2.3", "0.1.2.4", "0.1.2.5", "0.1.2.6",
	}
	for i, rawAddr := range addresses {
		addr := network.NewAddress(rawAddr)
		ipAddr, err := s.State.AddIPAddress(addr, "foobar")
		c.Assert(err, jc.ErrorIsNil)
		if i%2 == 1 {
			err = ipAddr.AllocateTo(s.MachineB.Id(), "wobble", "")
		} else {
			err = ipAddr.AllocateTo(s.MachineA.Id(), "wobble", "")
			c.Assert(err, jc.ErrorIsNil)
		}
	}
	// Two of the addresses start out allocated to this
	// machine which we destroy to test the handling of
	// addresses allocated to dead machines.
	err := s.MachineB.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.MachineB.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *workerSuite) waitForInitialDead(c *gc.C) {
	for a := common.ShortAttempt.Start(); a.Next(); {
		dead, err := s.State.DeadIPAddresses()
		c.Assert(err, jc.ErrorIsNil)
		if s.Enabled {
			// We expect dead IP addresses to be removed with
			// enabled Addresser worker.
			if len(dead) == 0 {
				break
			}
			if !a.HasNext() {
				c.Fatalf("timeout waiting for initial change (dead: %#v)", dead)
			}
		} else {
			// Without Addresser worker the dead IP addresses
			// will stay.
			if len(dead) == 0 {
				c.Fatal("IP addresses unexpectedly removed")
			}
		}
	}
}

func (s *workerSuite) waitForReleaseOp(c *gc.C) dummy.OpReleaseAddress {
	var releaseOp dummy.OpReleaseAddress
	var ok bool
	select {
	case op := <-s.OpsChan:
		releaseOp, ok = op.(dummy.OpReleaseAddress)
		c.Assert(ok, jc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while expecting release operation")
	}
	return releaseOp
}

func (s *workerSuite) assertNoReleaseOp(c *gc.C) {
	select {
	case op := <-s.OpsChan:
		_, ok := op.(dummy.OpReleaseAddress)
		if ok {
			c.Fatalf("received unexpected release operation")
		}
	case <-time.After(coretesting.ShortWait):
		return
	}
}

func (s *workerSuite) makeReleaseOp(digit int) dummy.OpReleaseAddress {
	return dummy.OpReleaseAddress{
		Env:        "dummyenv",
		InstanceId: "foo",
		SubnetId:   "foobar",
		Address:    network.NewAddress(fmt.Sprintf("0.1.2.%d", digit)),
	}
}

func (s *workerSuite) assertIPAddressLife(c *gc.C, value string, life state.Life) {
	ipAddr, err := s.State.IPAddress(value)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.Life(), gc.Equals, life)
}

func (s *workerSuite) assertIPAddressRemoved(c *gc.C, value string) {
	for a := common.ShortAttempt.Start(); a.Next(); {
		_, err := s.State.IPAddress(value)
		if errors.IsNotFound(err) {
			break
		}
		if !a.HasNext() {
			c.Fatalf("IP address not removed")
		}
	}
}

// workerEnabledSuite runs the test with the enabled address allocation.
type workerEnabledSuite struct {
	workerSuite
}

var _ = gc.Suite(&workerEnabledSuite{})

func (s *workerEnabledSuite) SetUpTest(c *gc.C) {
	s.workerSuite.Enabled = true

	s.workerSuite.SetUpTest(c)
}

func (s *workerEnabledSuite) TestWorkerIsStringsWorker(c *gc.C) {
	// In case of an environment able to allocte/deallocate
	// IP addresses the addresser worker is no finished worker.
	// See also TestWorkerIsFinishedWorker.
	c.Assert(s.Worker, gc.Not(gc.FitsTypeOf), worker.FinishedWorker{})
}

func (s *workerEnabledSuite) TestWorkerReleasesAlreadyDead(c *gc.C) {
	// Wait for releases of 0.1.2.4 and 0.1.2.6 first. It's
	// explicitely needed for this test for the assertion.
	op1 := s.waitForReleaseOp(c)
	op2 := s.waitForReleaseOp(c)

	expected := []dummy.OpReleaseAddress{s.makeReleaseOp(4), s.makeReleaseOp(6)}

	// The machines are dead, so ReleaseAddress should be called with
	// instance.UnknownId.
	expected[0].InstanceId = instance.UnknownId
	expected[1].InstanceId = instance.UnknownId

	c.Assert([]dummy.OpReleaseAddress{op1, op2}, jc.SameContents, expected)
}

func (s *workerEnabledSuite) TestWorkerIgnoresAliveAddresses(c *gc.C) {
	// Wait for releases of 0.1.2.4 and 0.1.2.6 first. Result is not needed.
	s.waitForReleaseOp(c)
	s.waitForReleaseOp(c)

	// Add a new alive address.
	addr := network.NewAddress("0.1.2.9")
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	err = ipAddr.AllocateTo(s.MachineA.Id(), "wobble", "")
	c.Assert(err, jc.ErrorIsNil)

	// Assert no ReleaseAddress call..
	s.assertNoReleaseOp(c)

	// The worker must not kill this address.
	for a := common.ShortAttempt.Start(); a.Next(); {
		s.assertIPAddressLife(c, "0.1.2.9", state.Alive)
	}
}

func (s *workerEnabledSuite) TestWorkerRemovesDeadAddress(c *gc.C) {
	// Wait for releases of 0.1.2.4 and 0.1.2.6 first. Result is not needed.
	s.waitForReleaseOp(c)
	s.waitForReleaseOp(c)

	// Kill IP address.
	addr, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	err = addr.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Wait for ReleaseAddress attempt.
	op := s.waitForReleaseOp(c)
	c.Assert(op, jc.DeepEquals, s.makeReleaseOp(3))

	// The address should have been removed from state.
	s.assertIPAddressRemoved(c, "0.1.2.3")
}

func (s *workerEnabledSuite) TestWorkerAcceptsBrokenRelease(c *gc.C) {
	// Wait for releases of 0.1.2.4 and 0.1.2.6 first. Result is not needed.
	s.waitForReleaseOp(c)
	s.waitForReleaseOp(c)

	// Break ReleaseAddress and kill IP address 0.1.2.3.
	s.AssertConfigParameterUpdated(c, "broken", "ReleaseAddress")

	ipAddr, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	err = ipAddr.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// The address should stay in state.
	s.assertIPAddressLife(c, "0.1.2.3", state.Dead)
	s.assertNoReleaseOp(c)

	// Make ReleaseAddress work again, it must be cleaned up then.
	s.AssertConfigParameterUpdated(c, "broken", "")

	// The address should have been removed from state.
	s.assertIPAddressRemoved(c, "0.1.2.3")
}

func (s *workerEnabledSuite) TestMachineRemovalTriggersWorker(c *gc.C) {
	// Wait for releases of 0.1.2.4 and 0.1.2.6 first. Result is not needed.
	s.waitForReleaseOp(c)
	s.waitForReleaseOp(c)

	// Add special test machine.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "really-fake", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Add a new alive address.
	addr := network.NewAddress("0.1.2.9")
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	err = ipAddr.AllocateTo(machine.Id(), "foo", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.InstanceId(), gc.Equals, instance.Id("foo"))

	// Ensure the alive address is not changed.
	for a := common.ShortAttempt.Start(); a.Next(); {
		s.assertIPAddressLife(c, ipAddr.Value(), state.Alive)
	}

	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.assertIPAddressLife(c, ipAddr.Value(), state.Dead)

	// Wait for ReleaseAddress attempt.
	op := s.waitForReleaseOp(c)
	c.Assert(op, jc.DeepEquals, s.makeReleaseOp(9))

	// The address should have been removed from state.
	s.assertIPAddressRemoved(c, "0.1.2.9")
}

// workerEnabledSuite runs the test with the disabled address allocation.
type workerDisabledSuite struct {
	workerSuite
}

var _ = gc.Suite(&workerDisabledSuite{})

func (s *workerDisabledSuite) SetUpTest(c *gc.C) {
	s.workerSuite.Enabled = false

	s.workerSuite.SetUpTest(c)
}

func (s *workerDisabledSuite) TestWorkerIsFinishedWorker(c *gc.C) {
	// In case of an environment not able to allocte/deallocate
	// IP addresses the worker is a finished worker.
	// See also TestWorkerIsStringsWorker.
	c.Assert(s.Worker, gc.FitsTypeOf, worker.FinishedWorker{})
}

func (s *workerDisabledSuite) TestWorkerIgnoresAddresses(c *gc.C) {
	// The worker must not kill these addresses.
	for a := common.ShortAttempt.Start(); a.Next(); {
		s.assertIPAddressLife(c, "0.1.2.3", state.Alive)
		s.assertIPAddressLife(c, "0.1.2.4", state.Dead)
		s.assertIPAddressLife(c, "0.1.2.5", state.Alive)
		s.assertIPAddressLife(c, "0.1.2.6", state.Dead)
	}
}
