// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/peergrouper"
)

type workerJujuConnSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&workerJujuConnSuite{})

func (s *workerJujuConnSuite) TestStartStop(c *gc.C) {
	st := peergrouper.NewFakeState()
	publish := func(apiServers [][]network.HostPort, instanceIds []instance.Id) error {
		return nil
	}
	w := peergrouper.NewWorker(st, peergrouper.PublisherFunc(publish))
	err := worker.Stop(w)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *workerJujuConnSuite) TestPublisherSetsAPIHostPorts(c *gc.C) {
	peergrouper.DoTestForIPv4AndIPv6(func(ipVersion peergrouper.TestIPVersion) {
		st := peergrouper.NewFakeState()
		peergrouper.InitState(c, st, 3, ipVersion)

		publishedAPIServers := make(chan [][]network.HostPort, 1)
		publish := func(apiServers [][]network.HostPort, instanceIds []instance.Id) error {
			publishedAPIServers <- apiServers
			return nil
		}

		w := peergrouper.NewWorker(st, peergrouper.PublisherFunc(publish))
		defer func() {
			c.Check(worker.Stop(w), gc.IsNil)
		}()

		select {
		case hps := <-publishedAPIServers:
			peergrouper.AssertAPIHostPorts(c, hps, peergrouper.ExpectedAPIHostPorts(3, ipVersion))
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for API server host-ports to be published")
		}

		// There should be only one publication.
		select {
		case <-publishedAPIServers:
			c.Fatalf("unexpected API server host-ports publication")
		case <-time.After(testing.ShortWait):
		}
	})
}
