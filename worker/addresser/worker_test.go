// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&workerSuite{})

type workerSuite struct {
	testing.JujuConnSuite
	machine  *state.Machine
	machine2 *state.Machine
}

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.AddressAllocation)
	// Unbreak dummy provider methods.
	s.AssertConfigParameterUpdated(c, "broken", "")

	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	s.machine = machine
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	// this machine will be destroyed after address creation to test the
	// handling of addresses for machines that have gone.
	machine2, err := s.State.AddMachine("quantal", state.JobHostUnits)
	s.machine2 = machine2
	c.Assert(err, jc.ErrorIsNil)

	s.createAddresses(c)
	s.State.StartSync()
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
			err = ipAddr.AllocateTo(s.machine2.Id(), "wobble")
		} else {
			err = ipAddr.AllocateTo(s.machine.Id(), "wobble")
			c.Assert(err, jc.ErrorIsNil)
		}

	}
	// Two of the addresses start out allocated to this
	// machine which we destroy to test the handling of
	// addresses allocated to dead machines.
	err := s.machine2.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine2.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func dummyListen() chan dummy.Operation {
	opsChan := make(chan dummy.Operation, 10)
	dummy.Listen(opsChan)
	return opsChan
}

func waitForReleaseOp(c *gc.C, opsChan chan dummy.Operation) dummy.OpReleaseAddress {
	var releaseOp dummy.OpReleaseAddress
	var ok bool
	select {
	case op := <-opsChan:
		releaseOp, ok = op.(dummy.OpReleaseAddress)
		c.Assert(ok, jc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while expecting operation")
	}
	return releaseOp
}

func makeReleaseOp(digit int) dummy.OpReleaseAddress {
	return dummy.OpReleaseAddress{
		Env:        "dummyenv",
		InstanceId: "foo",
		SubnetId:   "foobar",
		Address:    network.NewAddress(fmt.Sprintf("0.1.2.%d", digit)),
	}
}

func (s *workerSuite) assertStop(c *gc.C, w worker.Worker) {
	c.Assert(worker.Stop(w), jc.ErrorIsNil)
}

func (s *workerSuite) TestWorkerReleasesAlreadyDead(c *gc.C) {
	// we start with two dead addresses
	dead, err := s.State.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dead, gc.HasLen, 2)

	opsChan := dummyListen()

	w, err := addresser.NewWorker(s.State)
	c.Assert(err, jc.ErrorIsNil)
	defer s.assertStop(c, w)
	s.waitForInitialDead(c)

	op1 := waitForReleaseOp(c, opsChan)
	op2 := waitForReleaseOp(c, opsChan)
	expected := []dummy.OpReleaseAddress{makeReleaseOp(4), makeReleaseOp(6)}

	// The machines are dead, so ReleaseAddress should be called with
	// instance.UnknownId.
	expected[0].InstanceId = instance.UnknownId
	expected[1].InstanceId = instance.UnknownId
	c.Assert([]dummy.OpReleaseAddress{op1, op2}, jc.SameContents, expected)
}

func (s *workerSuite) waitForInitialDead(c *gc.C) {
	for a := common.ShortAttempt.Start(); a.Next(); {
		dead, err := s.State.DeadIPAddresses()
		c.Assert(err, jc.ErrorIsNil)
		if len(dead) == 0 {
			break
		}
		if !a.HasNext() {
			c.Fatalf("timeout waiting for initial change (dead: %#v)", dead)
		}
	}
}

func (s *workerSuite) TestWorkerIgnoresAliveAddresses(c *gc.C) {
	w, err := addresser.NewWorker(s.State)
	c.Assert(err, jc.ErrorIsNil)
	defer s.assertStop(c, w)
	s.waitForInitialDead(c)

	// Add a new alive address.
	addr := network.NewAddress("0.1.2.9")
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	err = ipAddr.AllocateTo(s.machine.Id(), "wobble")
	c.Assert(err, jc.ErrorIsNil)

	// The worker must not kill this address.
	for a := common.ShortAttempt.Start(); a.Next(); {
		ipAddr, err := s.State.IPAddress("0.1.2.9")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ipAddr.Life(), gc.Equals, state.Alive)
	}
}

func (s *workerSuite) TestWorkerRemovesDeadAddress(c *gc.C) {
	w, err := addresser.NewWorker(s.State)
	c.Assert(err, jc.ErrorIsNil)
	defer s.assertStop(c, w)
	s.waitForInitialDead(c)
	opsChan := dummyListen()

	addr, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	err = addr.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Wait for ReleaseAddress attempt.
	op := waitForReleaseOp(c, opsChan)
	c.Assert(op, jc.DeepEquals, makeReleaseOp(3))

	// The address should have been removed from state.
	for a := common.ShortAttempt.Start(); a.Next(); {
		_, err := s.State.IPAddress("0.1.2.3")
		if errors.IsNotFound(err) {
			break
		}
		if !a.HasNext() {
			c.Fatalf("IP address not removed")
		}
	}
}

func (s *workerSuite) TestMachineRemovalTriggersWorker(c *gc.C) {
	w, err := addresser.NewWorker(s.State)
	c.Assert(err, jc.ErrorIsNil)
	defer s.assertStop(c, w)
	s.waitForInitialDead(c)
	opsChan := dummyListen()

	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "really-fake", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.State.StartSync()

	addr, err := s.State.AddIPAddress(network.NewAddress("0.1.2.9"), "foobar")
	c.Assert(err, jc.ErrorIsNil)
	err = addr.AllocateTo(machine.Id(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.InstanceId(), gc.Equals, instance.Id("foo"))
	s.State.StartSync()

	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = addr.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Life(), gc.Equals, state.Dead)

	// Wait for ReleaseAddress attempt.
	op := waitForReleaseOp(c, opsChan)
	c.Assert(op, jc.DeepEquals, makeReleaseOp(9))

	// The address should have been removed from state.
	for a := common.ShortAttempt.Start(); a.Next(); {
		_, err := s.State.IPAddress("0.1.2.9")
		if errors.IsNotFound(err) {
			break
		}
		if !a.HasNext() {
			c.Fatalf("IP address not removed")
		}
	}
}

func (s *workerSuite) TestErrorKillsWorker(c *gc.C) {
	s.AssertConfigParameterUpdated(c, "broken", "ReleaseAddress")
	w, err := addresser.NewWorker(s.State)
	c.Assert(err, jc.ErrorIsNil)
	defer worker.Stop(w)

	// The worker should have died with an error.

	stopErr := make(chan error)
	go func() {
		w.Wait()
		stopErr <- worker.Stop(w)
	}()

	select {
	case err := <-stopErr:
		msg := "failed to release address .*: dummy.ReleaseAddress is broken"
		c.Assert(err, gc.ErrorMatches, msg)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("worker did not stop as expected")
	}

	// As we failed to release addresses they should not have been removed
	// from state.
	for _, digit := range []int{3, 4, 5, 6} {
		rawAddr := fmt.Sprintf("0.1.2.%d", digit)
		_, err := s.State.IPAddress(rawAddr)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *workerSuite) TestAddresserWithNoNetworkingEnviron(c *gc.C) {
	opsChan := dummyListen()
	w := addresser.NewWorkerWithReleaser(s.State, nil)
	defer s.assertStop(c, w)

	for {
		select {
		case <-opsChan:
			c.Fatalf("unexpected release op")
		case <-time.After(coretesting.ShortWait):
			return
		}
	}
}
