// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/status"
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
	minute := time.Minute
	s.fix = fixture{
		clock: testclock.NewClock(time.Now()),
		info: params.UndertakerModelInfoResult{
			Result: params.UndertakerModelInfo{
				Life:           "dying",
				DestroyTimeout: &minute,
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
		&params.Error{Code: params.CodeHasHostedModels},
		nil, // SetStatus
		&params.Error{Code: params.CodeModelNotEmpty},
		nil, // SetStatus
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
		"SetStatus",
		"ProcessDyingModel",
		"SetStatus",
		"ProcessDyingModel",
		"SetStatus",
		"Destroy",
		"RemoveModel",
	)
}

func (s *UndertakerSuite) TestProcessDyingModelErrorFatal(c *gc.C) {
	s.fix.errors = []error{
		nil, // ModelInfo
		nil, // SetStatus
		nil, // WatchModelResources,
		errors.New("nope"),
	}
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "nope")
	})
	stub.CheckCallNames(c,
		"ModelInfo",
		"SetStatus",
		"WatchModelResources",
		"ProcessDyingModel",
	)
}

func (s *UndertakerSuite) TestDestroyErrorFatal(c *gc.C) {
	s.fix.errors = []error{nil, nil, errors.New("pow")}
	s.fix.info.Result.Life = "dead"
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "cannot destroy cloud resources: pow")
	})
	stub.CheckCallNames(c, "ModelInfo", "SetStatus", "Destroy")
}

func (s *UndertakerSuite) TestDestroyErrorForced(c *gc.C) {
	s.fix.errors = []error{nil, nil, errors.New("pow")}
	s.fix.info.Result.Life = "dead"
	s.fix.info.Result.ForceDestroyed = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Assert(err, jc.ErrorIsNil)
	})
	// Removal continues despite the error calling destroy.
	stub.CheckCallNames(c, "ModelInfo", "SetStatus", "Destroy", "RemoveModel")
	// Logged the failed destroy call.
	s.fix.logger.stub.CheckCallNames(c, "Errorf")
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

func (s *UndertakerSuite) TestDestroyTimeout(c *gc.C) {
	notEmptyErr := &params.Error{Code: params.CodeModelNotEmpty}
	s.fix.errors = []error{nil, nil, nil, notEmptyErr, notEmptyErr, notEmptyErr, notEmptyErr, errors.Timeoutf("error")}
	s.fix.dirty = true
	s.fix.advance = 2 * time.Minute
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Assert(err, gc.ErrorMatches, ".* timeout")
	})
	// Depending on timing there can be 1 or more ProcessDyingModel calls.
	calls := stub.Calls()
	var callNames []string
	for i, call := range calls {
		if call.FuncName == "ProcessDyingModel" {
			continue
		}
		if i > 3 && call.FuncName == "SetStatus" {
			continue
		}
		callNames = append(callNames, call.FuncName)
	}
	c.Assert(callNames, jc.DeepEquals, []string{"ModelInfo", "SetStatus", "WatchModelResources"})
}

func (s *UndertakerSuite) TestDestroyTimeoutForce(c *gc.C) {
	s.fix.info.Result.ForceDestroyed = true
	s.fix.advance = 2 * time.Minute
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Assert(err, jc.ErrorIsNil)
	})
	// Depending on timing there can be 1 or more ProcessDyingModel calls.
	calls := stub.Calls()
	var callNames []string
	for _, call := range calls {
		if call.FuncName == "ProcessDyingModel" {
			continue
		}
		callNames = append(callNames, call.FuncName)
	}
	c.Assert(callNames, jc.DeepEquals, []string{"ModelInfo", "SetStatus", "WatchModelResources", "SetStatus", "Destroy", "RemoveModel"})
	s.fix.logger.stub.CheckNoCalls(c)
}

func (s *UndertakerSuite) TestEnvironDestroyTimeout(c *gc.C) {
	timeout := time.Millisecond
	s.fix.info.Result.DestroyTimeout = &timeout
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Assert(err, gc.ErrorMatches, "cannot destroy cloud resources: destroy model timeout")
	})
	// Depending on timing there can be 1 or more ProcessDyingModel calls.
	calls := stub.Calls()
	var callNames []string
	for _, call := range calls {
		if call.FuncName == "ProcessDyingModel" {
			continue
		}
		callNames = append(callNames, call.FuncName)
	}
	c.Assert(callNames, jc.DeepEquals, []string{"ModelInfo", "SetStatus", "WatchModelResources", "SetStatus", "Destroy"})
	s.fix.logger.stub.CheckNoCalls(c)
}

func (s *UndertakerSuite) TestEnvironDestroyTimeoutForce(c *gc.C) {
	timeout := time.Millisecond
	s.fix.info.Result.DestroyTimeout = &timeout
	s.fix.info.Result.ForceDestroyed = true
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		// We still get an error when the requested timeout is > 0.
		c.Assert(err, gc.ErrorMatches, "cannot destroy cloud resources: destroy model timeout")
	})
	// Depending on timing there can be 1 or more ProcessDyingModel calls.
	calls := stub.Calls()
	var callNames []string
	for _, call := range calls {
		if call.FuncName == "ProcessDyingModel" {
			continue
		}
		callNames = append(callNames, call.FuncName)
	}
	c.Assert(callNames, jc.DeepEquals, []string{"ModelInfo", "SetStatus", "WatchModelResources", "SetStatus", "Destroy"})
	s.fix.logger.stub.CheckCallNames(c, "Errorf")
}

func (s *UndertakerSuite) TestEnvironDestroyForceTimeoutZero(c *gc.C) {
	zero := time.Second * 0
	s.fix.info.Result.DestroyTimeout = &zero
	s.fix.info.Result.ForceDestroyed = true
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Assert(err, jc.ErrorIsNil)
	})
	stub.CheckCallNames(c, "ModelInfo", "SetStatus", "RemoveModel")
	s.fix.logger.stub.CheckNoCalls(c)
}
