// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/addresser"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var _ = gc.Suite(&workerSuite{})
var shortAttempt = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

type workerSuite struct {
	testing.JujuConnSuite
	machine *state.Machine
}

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
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
		addr := network.NewScopedAddress(rawAddr, network.ScopePublic)
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

func (s *workerSuite) TestWorkerReleasesAlreadyDead(c *gc.C) {
	w, err := addresser.NewWorker(s.State)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Assert(worker.Stop(w), gc.IsNil)
	}()

	dead, err := s.State.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(dead), gc.Equals, 2)
	for a := shortAttempt.Start(); a.Next(); {
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

func (s *workerSuite) TestWorkerRemovesDeadAddress(c *gc.C) {
	w, err := addresser.NewWorker(s.State)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Assert(worker.Stop(w), gc.IsNil)
	}()

	addr, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	err = addr.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	for a := shortAttempt.Start(); a.Next(); {
		_, err := s.State.IPAddress("0.1.2.3")
		if errors.IsNotFound(err) {
			break
		}
		if !a.HasNext() {
			c.Fail()
		}
	}
}
