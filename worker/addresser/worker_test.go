// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	stdtesting "testing"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
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
}

//func (s *workerSuite) SetUpTest(c *gc.C) {
//	s.AssertConfigParameterUpdated(c, "broken", []string{})
//}

func (s *workerSuite) createAddresses(c *gc.C) {
	addresses := [][]string{
		{"0.1.2.3", "wibble"},
		{"0.1.2.4", "wibble"},
		{"0.1.2.5", "wobble"},
		{"0.1.2.6", "wobble"},
	}
	for i, details := range addresses {
		addr := network.NewScopedAddress(details[0], network.ScopePublic)
		ipAddr, err := s.State.AddIPAddress(addr, "foobar")
		c.Assert(err, jc.ErrorIsNil)
		err = ipAddr.AllocateTo(details[1], "wobble")
		c.Assert(err, jc.ErrorIsNil)
		if i%2 == 1 {
			// two of the addresses start out Dead
			err = ipAddr.EnsureDead()
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

func (s *workerSuite) TestWorker(c *gc.C) {
	s.createAddresses(c)
	s.State.StartSync()
	w, err := addresser.NewWorker(s.State)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Assert(worker.Stop(w), gc.IsNil)
	}()

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
