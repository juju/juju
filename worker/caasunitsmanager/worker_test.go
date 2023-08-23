// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitsmanager_test

import (
	"github.com/juju/clock"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	message "github.com/juju/juju/internal/pubsub/agent"
	"github.com/juju/juju/worker/caasunitsmanager"
	"github.com/juju/juju/worker/caasunitsmanager/mocks"
)

var _ = gc.Suite(&workerSuite{})

type workerSuite struct{}

func (s *workerSuite) newWorker(c *gc.C, hub caasunitsmanager.Hub) worker.Worker {
	config := caasunitsmanager.Config{
		Logger: loggo.GetLogger("test"),
		Clock:  clock.WallClock,
		Hub:    hub,
	}
	w, err := caasunitsmanager.NewWorker(config)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *workerSuite) TestStartStop(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hub := mocks.NewMockHub(ctrl)

	unsubStopCalled := false
	unsubStartCalled := false
	unsubStatusCalled := false
	unsubStop := func() { unsubStopCalled = true }
	unsubStart := func() { unsubStartCalled = true }
	unsubStatus := func() { unsubStatusCalled = true }

	gomock.InOrder(
		hub.EXPECT().Subscribe(message.StopUnitTopic, gomock.Any()).Return(unsubStop),
		hub.EXPECT().Subscribe(message.StartUnitTopic, gomock.Any()).Return(unsubStart),
		hub.EXPECT().Subscribe(message.UnitStatusTopic, gomock.Any()).Return(unsubStatus),

		hub.EXPECT().Publish(
			message.StopUnitResponseTopic,
			message.StartStopResponse{"error": `stop units for {[minio/0]} not supported`},
		),
		hub.EXPECT().Publish(
			message.StartUnitResponseTopic,
			message.StartStopResponse{"error": `start units for {[minio/0]} not supported`},
		),

		hub.EXPECT().Publish(
			message.UnitStatusResponseTopic,
			message.Status{"error": `units status not supported`},
		),
	)

	w := s.newWorker(c, hub)
	m := w.(caasunitsmanager.Manager)
	m.StopUnitRequest(message.StopUnitTopic, message.Units{Names: []string{"minio/0"}})
	m.StartUnitRequest(message.StartUnitTopic, message.Units{Names: []string{"minio/0"}})
	m.UnitStatusRequest(message.UnitStatusTopic, message.Units{Names: []string{"minio/0"}})

	w.Kill()
	err := w.Wait()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsubStopCalled, jc.IsTrue)
	c.Assert(unsubStartCalled, jc.IsTrue)
	c.Assert(unsubStatusCalled, jc.IsTrue)
}
