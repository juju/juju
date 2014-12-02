// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/peergrouper"
)

type workerJujuConnSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&workerJujuConnSuite{})

func (s *workerJujuConnSuite) TestStartStop(c *gc.C) {
	w, err := peergrouper.New(s.State)
	c.Assert(err, jc.ErrorIsNil)
	err = worker.Stop(w)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *workerJujuConnSuite) TestPublisherSetsAPIHostPorts(c *gc.C) {
	peergrouper.DoTestForIPv4AndIPv6(func(ipVersion peergrouper.TestIPVersion) {
		st := peergrouper.NewFakeState()
		peergrouper.InitState(c, st, 3, ipVersion)

		watcher := s.State.WatchAPIHostPorts()
		cwatch := statetesting.NewNotifyWatcherC(c, s.State, watcher)
		cwatch.AssertOneChange()

		statePublish := peergrouper.NewPublisher(s.State, false)

		// Wrap the publisher so that we can call StartSync immediately
		// after the publishAPIServers method is called.
		publish := func(apiServers [][]network.HostPort, instanceIds []instance.Id) error {
			err := statePublish.PublishAPIServers(apiServers, instanceIds)
			s.State.StartSync()
			return err
		}

		w := peergrouper.NewWorker(st, peergrouper.PublisherFunc(publish))
		defer func() {
			c.Check(worker.Stop(w), gc.IsNil)
		}()

		cwatch.AssertOneChange()
		hps, err := s.State.APIHostPorts()
		c.Assert(err, jc.ErrorIsNil)
		peergrouper.AssertAPIHostPorts(c, hps, peergrouper.ExpectedAPIHostPorts(3, ipVersion))
	})
}

func (s *workerJujuConnSuite) TestPublisherRejectsNoServers(c *gc.C) {
	peergrouper.DoTestForIPv4AndIPv6(func(ipVersion peergrouper.TestIPVersion) {
		st := peergrouper.NewFakeState()
		peergrouper.InitState(c, st, 3, ipVersion)
		statePublish := peergrouper.NewPublisher(s.State, false)
		err := statePublish.PublishAPIServers(nil, nil)
		c.Assert(err, gc.ErrorMatches, "no api servers specified")
	})
}
