// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/workertest"
)

const RIPTime = 18 * time.Hour

// UndertakerSuite is *not* complete. But it's a lot more so
// than it was before, and should be much easier to extend.
type UndertakerSuite struct {
	testing.IsolationSuite
	fix fixture
}

var _ = gc.Suite(&UndertakerSuite{})

func (s *UndertakerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.fix = fixture{
		info: params.UndertakerModelInfoResult{
			Result: params.UndertakerModelInfo{
				Life: "dying",
			},
		},
	}
}

func (s *UndertakerSuite) TestAliveError(c *gc.C) {
	s.fix.info.Result.Life = "alive"
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker, _ *coretesting.Clock) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "model still alive")
	})
	stub.CheckCallNames(c, "ModelInfo")
}

func (s *UndertakerSuite) TestAlreadyDeadTimeRecordedWaits(c *gc.C) {
	halfTime := RIPTime / 2
	diedAt := time.Now().Add(-halfTime)
	s.fix.info.Result.Life = "dead"
	s.fix.info.Result.TimeOfDeath = &diedAt
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		waitAlarm(c, clock)
		clock.Advance(halfTime - time.Second)
		workertest.CheckAlive(c, w)
	})
	stub.CheckCallNames(c, "ModelInfo", "SetStatus", "Destroy", "SetStatus")
}

func (s *UndertakerSuite) TestAlreadyDeadTimeRecordedFinishes(c *gc.C) {
	halfTime := RIPTime / 2
	diedAt := time.Now().Add(-halfTime)
	s.fix.info.Result.Life = "dead"
	s.fix.info.Result.TimeOfDeath = &diedAt
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		waitAlarm(c, clock)
		clock.Advance(halfTime)
		workertest.CheckKilled(c, w)
	})
	stub.CheckCallNames(c, "ModelInfo", "SetStatus", "Destroy", "SetStatus", "RemoveModel")
}

func (s *UndertakerSuite) TestAlreadyDeadTimeMissingWaits(c *gc.C) {
	s.fix.info.Result.Life = "dead"
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		waitAlarm(c, clock)
		clock.Advance(RIPTime - time.Second)
		workertest.CheckAlive(c, w)
	})
	stub.CheckCallNames(c, "ModelInfo", "SetStatus", "Destroy", "SetStatus")
}

func (s *UndertakerSuite) TestAlreadyDeadTimeMissingFinishes(c *gc.C) {
	s.fix.info.Result.Life = "dead"
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		waitAlarm(c, clock)
		clock.Advance(RIPTime)
		workertest.CheckKilled(c, w)
	})
	stub.CheckCallNames(c, "ModelInfo", "SetStatus", "Destroy", "SetStatus", "RemoveModel")
}

func (s *UndertakerSuite) TestImmediateSuccess(c *gc.C) {
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		waitAlarm(c, clock)
		clock.Advance(RIPTime - time.Second)
		workertest.CheckAlive(c, w)
	})
	stub.CheckCallNames(c,
		"ModelInfo",
		"SetStatus",
		"WatchModelResources",
		"ProcessDyingModel",
		"SetStatus",
		"Destroy",
		"SetStatus",
	)
}

func (s *UndertakerSuite) TestSetStatusDestroying(c *gc.C) {
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		waitAlarm(c, clock)
		clock.Advance(RIPTime - time.Second)
		workertest.CheckAlive(c, w)
	})
	stub.CheckCall(
		c, 1, "SetStatus", status.StatusDestroying,
		"cleaning up cloud resources", map[string]interface{}(nil),
	)
	stub.CheckCall(
		c, 4, "SetStatus", status.StatusDestroying,
		"tearing down cloud environment", map[string]interface{}(nil),
	)
}

func (s *UndertakerSuite) TestSetStatusArchived(c *gc.C) {
	s.fix.info.Result.Life = "dead"
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		waitAlarm(c, clock)
		clock.Advance(RIPTime - time.Second)
		workertest.CheckAlive(c, w)
	})
	stub.CheckCall(
		c, 3, "SetStatus", status.StatusArchived,
		"", map[string]interface{}(nil),
	)
}

func (s *UndertakerSuite) TestControllerStopsWhenModelDead(c *gc.C) {
	s.fix.info.Result.IsSystem = true
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		workertest.CheckKilled(c, w)
	})
	stub.CheckCallNames(c,
		"ModelInfo",
		"SetStatus",
		"WatchModelResources",
		"ProcessDyingModel",
	)
}

func (s *UndertakerSuite) TestFinalRemove(c *gc.C) {
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		waitAlarm(c, clock)
		clock.Advance(RIPTime)
		workertest.CheckKilled(c, w)
	})
	stub.CheckCallNames(c,
		"ModelInfo",
		"SetStatus",
		"WatchModelResources",
		"ProcessDyingModel",
		"SetStatus",
		"Destroy",
		"SetStatus",
		"RemoveModel",
	)
}

func (s *UndertakerSuite) TestModelInfoErrorFatal(c *gc.C) {
	s.fix.errors = []error{errors.New("pow")}
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "pow")
	})
	stub.CheckCallNames(c, "ModelInfo")
}

func (s *UndertakerSuite) TestWatchModelResourcesErrorFatal(c *gc.C) {
	s.fix.errors = []error{nil, nil, errors.New("pow")}
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "pow")
	})
	stub.CheckCallNames(c, "ModelInfo", "SetStatus", "WatchModelResources")
}

func (s *UndertakerSuite) TestProcessDyingModelErrorRetried(c *gc.C) {
	s.fix.errors = []error{
		nil, // ModelInfo
		nil, // SetStatus
		nil, // WatchModelResources,
		errors.New("meh, will retry"),  // ProcessDyingModel,
		errors.New("will retry again"), // ProcessDyingModel,
		nil, // ProcessDyingModel,
		nil, // SetStatus
		nil, // Destroy,
		nil, // SetStatus
	}
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		waitAlarm(c, clock)
		workertest.CheckAlive(c, w)
	})
	stub.CheckCallNames(c,
		"ModelInfo",
		"SetStatus",
		"WatchModelResources",
		"ProcessDyingModel",
		"ProcessDyingModel",
		"ProcessDyingModel",
		"SetStatus",
		"Destroy",
		"SetStatus",
	)
}

func (s *UndertakerSuite) TestDestroyErrorFatal(c *gc.C) {
	s.fix.errors = []error{nil, nil, errors.New("pow")}
	s.fix.info.Result.Life = "dead"
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "pow")
	})
	stub.CheckCallNames(c, "ModelInfo", "SetStatus", "Destroy")
}

func (s *UndertakerSuite) TestRemoveModelErrorFatal(c *gc.C) {
	s.fix.errors = []error{nil, nil, nil, nil, errors.New("pow")}
	s.fix.info.Result.Life = "dead"
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker, clock *coretesting.Clock) {
		waitAlarm(c, clock)
		clock.Advance(RIPTime)
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "cannot remove model: pow")
	})
	stub.CheckCallNames(c, "ModelInfo", "SetStatus", "Destroy", "SetStatus", "RemoveModel")
}

func waitAlarm(c *gc.C, clock *coretesting.Clock) {
	select {
	case <-clock.Alarms():
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SUT to use clock")
	}
}
