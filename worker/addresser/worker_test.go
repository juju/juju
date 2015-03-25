// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/addresser"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var _ = gc.Suite(&workerSuite{})

type workerSuite struct {
	testing.JujuConnSuite
	machine *state.Machine
}

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	// unbreak provider methods
	s.AssertConfigParameterUpdated(c, "broken", "")

	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	s.machine = machine
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetProvisioned("foo", "fake_nonce", nil)
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
		err = ipAddr.AllocateTo(s.machine.Id(), "wobble")
		c.Assert(err, jc.ErrorIsNil)
		if i%2 == 1 {
			// two of the addresses start out Dead
			err = ipAddr.EnsureDead()
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

func dummyListen() chan dummy.Operation {
	opsChan := make(chan dummy.Operation, 10)
	dummy.Listen(opsChan)
	return opsChan
}

func waitForReleaseOp(c *gc.C, opsChan chan dummy.Operation) dummy.OpReleaseAddress {
	op := <-opsChan
	releaseOp, ok := op.(dummy.OpReleaseAddress)
	c.Assert(ok, jc.IsTrue)
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

func (s *workerSuite) TestWorkerReleasesAlreadyDead(c *gc.C) {
	// we start with two dead addresses
	dead, err := s.State.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(dead), gc.Equals, 2)

	opsChan := dummyListen()

	w, err := addresser.NewWorker(s.State)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Assert(worker.Stop(w), gc.IsNil)
	}()
	s.waitForInitialDead(c)

	ops := []dummy.OpReleaseAddress{}
	for i := 1; i <= 10; i++ {
		select {
		case op := <-opsChan:
			releaseOp, ok := op.(dummy.OpReleaseAddress)
			if !ok {
				c.Fail()
			}
			ops = append(ops, releaseOp)
		default:
		}
	}

	var found4 bool
	var found6 bool
	for _, anOp := range ops {
		if anOp.Address.Value == "0.1.2.4" {
			found4 = true
		} else if anOp.Address.Value == "0.1.2.6" {
			found6 = true
		}
	}
	c.Assert(found4, jc.IsTrue)
	c.Assert(found6, jc.IsTrue)

}

func (s *workerSuite) waitForInitialDead(c *gc.C) {
	for a := common.ShortAttempt.Start(); a.Next(); {
		dead, err := s.State.DeadIPAddresses()
		c.Assert(err, jc.ErrorIsNil)
		if len(dead) == 0 {
			break
		}
		if !a.HasNext() {
			c.Fail()
		}
	}
}

func (s *workerSuite) TestWorkerIgnoresAliveAddresses(c *gc.C) {
	w, err := addresser.NewWorker(s.State)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Assert(worker.Stop(w), gc.IsNil)
	}()
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
	defer func() {
		c.Assert(worker.Stop(w), gc.IsNil)
	}()
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
			c.Fail()
		}
	}
}

func (s *workerSuite) TestWorkerHandlesProviderError(c *gc.C) {
	w, err := addresser.NewWorker(s.State)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Assert(worker.Stop(w), gc.IsNil)
	}()
	s.waitForInitialDead(c)
	// now break the ReleaseAddress provider method
	s.AssertConfigParameterUpdated(c, "broken", "ReleaseAddress")

	opsChan := dummyListen()

	addr, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	err = addr.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// wait for ReleaseAddress attempt
	op := waitForReleaseOp(c, opsChan)
	c.Assert(op, jc.DeepEquals, makeReleaseOp(3))

	// As we failed to release the address it should not have been removed
	// from state.
	_, err = s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *workerSuite) TestWorkerErrorOnStartKillsWorker(c *gc.C) {
	s.AssertConfigParameterUpdated(c, "broken", "ReleaseAddress")
	w, err := addresser.NewWorker(s.State)
	c.Assert(err, jc.ErrorIsNil)

	// The worker should have died with an error.
	w.Wait()
	msg := "failed to release address .*: dummy.ReleaseAddress is broken"
	c.Assert(worker.Stop(w), gc.ErrorMatches, msg)
}
