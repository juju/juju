// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	environscontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/worker/undertaker"
	"github.com/juju/juju/rpc/params"
)

// OldUndertakerSuite is *not* complete. But it's a lot more so
// than it was before, and should be much easier to extend.
type OldUndertakerSuite struct {
	testing.IsolationSuite
	fix fixture
}

var _ = gc.Suite(&OldUndertakerSuite{})

func (s *OldUndertakerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	minute := time.Minute
	s.fix = fixture{
		clock: testclock.NewDilatedWallClock(10 * time.Millisecond),
		info: params.UndertakerModelInfoResult{
			Result: params.UndertakerModelInfo{
				Life:           "dying",
				DestroyTimeout: &minute,
			},
		},
	}
}

func (s *OldUndertakerSuite) TestAliveError(c *gc.C) {
	s.fix.info.Result.Life = "alive"
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "model still alive")
	})
	stub.CheckCallNames(c, "WatchModel", "ModelInfo")
}

func (s *OldUndertakerSuite) TestAlreadyDeadRemoves(c *gc.C) {
	s.fix.info.Result.Life = "dead"
	stub := s.fix.run(c, func(w worker.Worker) {
		workertest.CheckKilled(c, w)
	})
	stub.CheckCallNames(c, "WatchModel", "ModelInfo", "SetStatus", "ModelConfig", "CloudSpec", "Destroy", "RemoveModelSecrets", "RemoveModel")
}

func (s *OldUndertakerSuite) TestDyingDeadRemoved(c *gc.C) {
	stub := s.fix.run(c, func(w worker.Worker) {
		workertest.CheckKilled(c, w)
	})
	stub.CheckCallNames(c,
		"WatchModel",
		"ModelInfo",
		"SetStatus",
		"WatchModelResources",
		"ProcessDyingModel",
		"SetStatus",
		"ModelConfig",
		"CloudSpec",
		"Destroy",
		"RemoveModelSecrets",
		"RemoveModel",
	)
}

func (s *OldUndertakerSuite) TestSetStatusDestroying(c *gc.C) {
	stub := s.fix.run(c, func(w worker.Worker) {
		workertest.CheckKilled(c, w)
	})
	stub.CheckCallNames(c,
		"WatchModel", "ModelInfo", "SetStatus", "WatchModelResources", "ProcessDyingModel",
		"SetStatus", "ModelConfig", "CloudSpec", "Destroy", "RemoveModelSecrets", "RemoveModel")
	stub.CheckCall(
		c, 2, "SetStatus", status.Destroying,
		"cleaning up cloud resources", map[string]interface{}(nil),
	)
	stub.CheckCall(
		c, 5, "SetStatus", status.Destroying,
		"tearing down cloud environment", map[string]interface{}(nil),
	)
}

func (s *OldUndertakerSuite) TestControllerStopsWhenModelDead(c *gc.C) {
	s.fix.info.Result.IsSystem = true
	stub := s.fix.run(c, func(w worker.Worker) {
		workertest.CheckKilled(c, w)
	})
	stub.CheckCallNames(c,
		"WatchModel",
		"ModelInfo",
		"SetStatus",
		"WatchModelResources",
		"ProcessDyingModel",
	)
}

func (s *OldUndertakerSuite) TestModelInfoErrorFatal(c *gc.C) {
	s.fix.errors = []error{nil, errors.New("pow")}
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "pow")
	})
	stub.CheckCallNames(c, "WatchModel", "ModelInfo")
}

func (s *OldUndertakerSuite) TestWatchModelResourcesErrorFatal(c *gc.C) {
	s.fix.errors = []error{nil, nil, nil, errors.New("pow")}
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "proccesing model death: pow")
	})
	stub.CheckCallNames(c, "WatchModel", "ModelInfo", "SetStatus", "WatchModelResources")
}

func (s *OldUndertakerSuite) TestProcessDyingModelErrorRetried(c *gc.C) {
	s.fix.errors = []error{
		nil, // WatchModel
		nil, // ModelInfo
		nil, // SetStatus
		nil, // WatchModelResources,
		&params.Error{Code: params.CodeHasHostedModels},
		nil, // SetStatus
		&params.Error{Code: params.CodeModelNotEmpty},
		nil, // SetStatus
		nil, // ProcessDyingModel,
		nil, // SetStatus
		nil, // ModelConfig
		nil, // CloudSpec
		nil, // Destroy,
		nil, // RemoveModel
	}
	stub := s.fix.run(c, func(w worker.Worker) {
		workertest.CheckKilled(c, w)
	})
	stub.CheckCallNames(c,
		"WatchModel",
		"ModelInfo",
		"SetStatus",
		"WatchModelResources",
		"ProcessDyingModel",
		"SetStatus",
		"ProcessDyingModel",
		"SetStatus",
		"ProcessDyingModel",
		"SetStatus",
		"ModelConfig",
		"CloudSpec",
		"Destroy",
		"RemoveModelSecrets",
		"RemoveModel",
	)
}

func (s *OldUndertakerSuite) TestProcessDyingModelErrorFatal(c *gc.C) {
	s.fix.errors = []error{
		nil, // WatchModel
		nil, // ModelInfo
		nil, // SetStatus
		nil, // WatchModelResources,
		errors.New("nope"),
	}
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "proccesing model death: nope")
	})
	stub.CheckCallNames(c,
		"WatchModel",
		"ModelInfo",
		"SetStatus",
		"WatchModelResources",
		"ProcessDyingModel",
	)
}

func (s *OldUndertakerSuite) TestDestroyErrorFatal(c *gc.C) {
	s.fix.errors = []error{nil, nil, nil, nil, nil, errors.New("pow")}
	s.fix.info.Result.Life = "dead"
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "cannot destroy cloud resources: process destroy environ: pow")
	})
	stub.CheckCallNames(c, "WatchModel", "ModelInfo", "SetStatus", "ModelConfig", "CloudSpec", "Destroy")
}

func (s *OldUndertakerSuite) TestDestroyErrorForced(c *gc.C) {
	s.fix.errors = []error{nil, nil, nil, nil, nil, errors.New("pow")}
	s.fix.info.Result.Life = "dead"
	s.fix.info.Result.ForceDestroyed = true
	destroyTimeout := 500 * time.Millisecond
	s.fix.info.Result.DestroyTimeout = &destroyTimeout
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Assert(err, jc.ErrorIsNil)
	})
	// Removal continues despite the error calling destroy.
	mainCalls, destroyCloudCalls := s.sortCalls(c, stub)
	c.Assert(mainCalls, jc.DeepEquals, []string{"WatchModel", "ModelInfo", "SetStatus", "RemoveModelSecrets", "RemoveModel"})
	c.Assert(destroyCloudCalls, jc.DeepEquals, []string{"ModelConfig", "CloudSpec", "Destroy"})
	// Logged the failed destroy call.
	s.fix.logger.stub.CheckCallNames(c, "Errorf")
}

func (s *OldUndertakerSuite) TestRemoveModelErrorFatal(c *gc.C) {
	s.fix.errors = []error{nil, nil, nil, nil, nil, nil, nil, errors.New("pow")}
	s.fix.info.Result.Life = "dead"
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "cannot remove model: pow")
	})
	mainCalls, destroyCloudCalls := s.sortCalls(c, stub)
	c.Assert(mainCalls, jc.DeepEquals, []string{"WatchModel", "ModelInfo", "SetStatus", "RemoveModelSecrets", "RemoveModel"})
	c.Assert(destroyCloudCalls, jc.DeepEquals, []string{"ModelConfig", "CloudSpec", "Destroy"})
}

func (s *OldUndertakerSuite) TestDestroyTimeout(c *gc.C) {
	notEmptyErr := &params.Error{Code: params.CodeModelNotEmpty}
	s.fix.errors = []error{nil, nil, nil, nil, notEmptyErr, notEmptyErr, notEmptyErr, notEmptyErr, errors.Timeoutf("error")}
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
		if i > 4 && call.FuncName == "SetStatus" {
			continue
		}
		callNames = append(callNames, call.FuncName)
	}
	c.Assert(callNames, jc.DeepEquals, []string{"WatchModel", "ModelInfo", "SetStatus", "WatchModelResources"})
}

func (s *OldUndertakerSuite) TestDestroyTimeoutForce(c *gc.C) {
	s.fix.info.Result.ForceDestroyed = true
	s.fix.advance = 2 * time.Minute
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Assert(err, jc.ErrorIsNil)
	})
	mainCalls, destroyCloudCalls := s.sortCalls(c, stub)
	c.Assert(mainCalls, jc.DeepEquals, []string{"WatchModel", "ModelInfo", "SetStatus", "WatchModelResources", "ProcessDyingModel", "SetStatus", "RemoveModelSecrets", "RemoveModel"})
	c.Assert(destroyCloudCalls, jc.DeepEquals, []string{"ModelConfig", "CloudSpec", "Destroy"})
	s.fix.logger.stub.CheckNoCalls(c)
}

func (s *OldUndertakerSuite) TestEnvironDestroyTimeout(c *gc.C) {
	timeout := time.Millisecond
	s.fix.info.Result.DestroyTimeout = &timeout
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Assert(err, jc.ErrorIsNil)
	})
	mainCalls, destroyCloudCalls := s.sortCalls(c, stub)
	c.Assert(mainCalls, jc.DeepEquals, []string{"WatchModel", "ModelInfo", "SetStatus", "WatchModelResources", "ProcessDyingModel", "SetStatus", "RemoveModelSecrets", "RemoveModel"})
	c.Assert(destroyCloudCalls, jc.DeepEquals, []string{"ModelConfig", "CloudSpec", "Destroy"})
	s.fix.logger.stub.CheckCall(c, 0, "Warningf", "timeout ignored for graceful model destroy", []interface{}(nil))
}

func (s *OldUndertakerSuite) TestEnvironDestroyTimeoutForce(c *gc.C) {
	timeout := time.Second
	s.fix.info.Result.DestroyTimeout = &timeout
	s.fix.info.Result.ForceDestroyed = true
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Assert(err, jc.ErrorIsNil)
	})
	mainCalls, destroyCloudCalls := s.sortCalls(c, stub)
	c.Assert(mainCalls, jc.DeepEquals, []string{"WatchModel", "ModelInfo", "SetStatus", "WatchModelResources", "ProcessDyingModel", "SetStatus", "RemoveModelSecrets", "RemoveModel"})
	c.Assert(destroyCloudCalls, jc.DeepEquals, []string{"ModelConfig", "CloudSpec", "Destroy"})
}

func (s *OldUndertakerSuite) sortCalls(c *gc.C, stub *testing.Stub) (mainCalls []string, destroyCloudCalls []string) {
	calls := stub.Calls()
	for _, call := range calls {
		switch call.FuncName {
		case "ModelConfig", "CloudSpec", "Destroy":
			destroyCloudCalls = append(destroyCloudCalls, call.FuncName)
		default:
			mainCalls = append(mainCalls, call.FuncName)
		}
	}
	return
}

func (s *OldUndertakerSuite) TestEnvironDestroyForceTimeoutZero(c *gc.C) {
	zero := time.Second * 0
	s.fix.info.Result.DestroyTimeout = &zero
	s.fix.info.Result.ForceDestroyed = true
	s.fix.dirty = true
	stub := s.fix.run(c, func(w worker.Worker) {
		err := workertest.CheckKilled(c, w)
		c.Assert(err, jc.ErrorIsNil)
	})
	stub.CheckCallNames(c, "WatchModel", "ModelInfo", "RemoveModelSecrets", "RemoveModel")
	s.fix.logger.stub.CheckNoCalls(c)
}

type UndertakerSuite struct{}

var _ = gc.Suite(&UndertakerSuite{})

func (s *UndertakerSuite) TestExitOnModelChanged(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	controllerUUID := utils.MustNewUUID().String()

	facade := NewMockFacade(ctrl)
	facade.EXPECT().SetStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	modelChanged := make(chan struct{}, 1)
	modelChanged <- struct{}{}
	modelResources := make(chan struct{}, 1)
	modelResources <- struct{}{}
	facade.EXPECT().WatchModel().DoAndReturn(func() (watcher.NotifyWatcher, error) {
		return watchertest.NewMockNotifyWatcher(modelChanged), nil
	})
	facade.EXPECT().WatchModelResources().DoAndReturn(func() (watcher.NotifyWatcher, error) {
		return watchertest.NewMockNotifyWatcher(modelResources), nil
	})
	facade.EXPECT().ModelConfig().Return(nil, nil)

	gomock.InOrder(
		facade.EXPECT().ModelInfo().Return(params.UndertakerModelInfoResult{
			Result: params.UndertakerModelInfo{
				Life:           life.Dying,
				ForceDestroyed: false,
				ControllerUUID: controllerUUID,
			},
		}, nil),
		facade.EXPECT().ProcessDyingModel().Return(nil),
		facade.EXPECT().CloudSpec().DoAndReturn(func() (cloudspec.CloudSpec, error) {
			modelChanged <- struct{}{}
			return cloudspec.CloudSpec{}, nil
		}),
		facade.EXPECT().ModelInfo().Return(params.UndertakerModelInfoResult{
			Result: params.UndertakerModelInfo{
				Life:           life.Dying,
				ForceDestroyed: true, // changed from false to true to cause worker to exit.
				ControllerUUID: controllerUUID,
			},
		}, nil),
	)

	credentialAPI := NewMockCredentialAPI(ctrl)

	w, err := undertaker.NewUndertaker(undertaker.Config{
		Facade:        facade,
		CredentialAPI: credentialAPI,
		Logger:        loggo.GetLogger("test"),
		Clock:         testclock.NewDilatedWallClock(testing.ShortWait),
		NewCloudDestroyerFunc: func(ctx context.Context, op environs.OpenParams) (environs.CloudDestroyer, error) {
			return &waitDestroyer{}, nil
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckKilled(c, w)

	err = w.Wait()
	c.Assert(err, gc.ErrorMatches, "model destroy parameters changed")
}

type waitDestroyer struct {
	environs.Environ
}

func (w *waitDestroyer) Destroy(ctx environscontext.ProviderCallContext) error {
	<-ctx.Done()
	return ctx.Err()
}
