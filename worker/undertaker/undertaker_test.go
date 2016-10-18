// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/status"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/workertest"
)

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
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "model still alive")
	})
	stub.CheckCallNames(c, "ModelInfo")
}

func (s *UndertakerSuite) TestAlreadyDeadRemoves(c *gc.C) {
	s.fix.info.Result.Life = "dead"
	stub := s.fix.run(c, func(w worker.Worker) {
		workertest.CheckKilled(c, w)
	})
	stub.CheckCallNames(c, "ModelInfo", "SetStatus", "Destroy", "RemoveModel")
}

func (s *UndertakerSuite) TestDyingDeadRemoved(c *gc.C) {
	stub := s.fix.run(c, func(w worker.Worker) {
		workertest.CheckKilled(c, w)
	})
	stub.CheckCallNames(c,
		"ModelInfo",
		"SetStatus",
		"WatchModelResources",
		"ProcessDyingModel",
		"SetStatus",
		"Destroy",
		"RemoveModel",
	)
}

func (s *UndertakerSuite) TestSetStatusDestroying(c *gc.C) {
	stub := s.fix.run(c, func(w worker.Worker) {
		workertest.CheckKilled(c, w)
	})
	stub.CheckCall(
		c, 1, "SetStatus", status.Destroying,
		"cleaning up cloud resources", map[string]interface{}(nil),
	)
	stub.CheckCall(
		c, 4, "SetStatus", status.Destroying,
		"tearing down cloud environment", map[string]interface{}(nil),
	)
}

func (s *UndertakerSuite) TestControllerStopsWhenModelDead(c *gc.C) {
	s.fix.info.Result.IsSystem = true
	stub := s.fix.run(c, func(w worker.Worker) {
		workertest.CheckKilled(c, w)
	})
	stub.CheckCallNames(c,
		"ModelInfo",
		"SetStatus",
		"WatchModelResources",
		"ProcessDyingModel",
	)
}

func (s *UndertakerSuite) TestModelInfoErrorFatal(c *gc.C) {
	s.fix.errors = []error{errors.New("pow")}
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "pow")
	})
	stub.CheckCallNames(c, "ModelInfo")
}

func (s *UndertakerSuite) TestWatchModelResourcesErrorFatal(c *gc.C) {
	s.fix.errors = []error{nil, nil, errors.New("pow")}
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
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
		nil, // RemoveModel
	}
	stub := s.fix.run(c, func(w worker.Worker) {
		workertest.CheckKilled(c, w)
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
		"RemoveModel",
	)
}

func (s *UndertakerSuite) TestDestroyErrorFatal(c *gc.C) {
	s.fix.errors = []error{nil, nil, errors.New("pow")}
	s.fix.info.Result.Life = "dead"
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "pow")
	})
	stub.CheckCallNames(c, "ModelInfo", "SetStatus", "Destroy")
}

func (s *UndertakerSuite) TestRemoveModelErrorFatal(c *gc.C) {
	s.fix.errors = []error{nil, nil, nil, errors.New("pow")}
	s.fix.info.Result.Life = "dead"
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "cannot remove model: pow")
	})
	stub.CheckCallNames(c, "ModelInfo", "SetStatus", "Destroy", "RemoveModel")
}
