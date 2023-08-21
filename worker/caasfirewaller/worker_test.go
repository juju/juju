// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"time"

	"github.com/juju/charm/v11"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasfirewaller"
)

type WorkerSuite struct {
	testing.IsolationSuite

	config            caasfirewaller.Config
	applicationGetter mockApplicationGetter
	serviceExposer    mockServiceExposer
	lifeGetter        mockLifeGetter
	charmGetter       mockCharmGetter

	applicationChanges chan []string
	appExposedChange   chan struct{}
	serviceExposed     chan struct{}
	serviceUnexposed   chan struct{}
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.applicationChanges = make(chan []string)
	s.appExposedChange = make(chan struct{})
	s.serviceExposed = make(chan struct{})
	s.serviceUnexposed = make(chan struct{})

	s.applicationGetter = mockApplicationGetter{
		allWatcher: watchertest.NewMockStringsWatcher(s.applicationChanges),
		appWatcher: watchertest.NewMockNotifyWatcher(s.appExposedChange),
	}
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.applicationGetter.allWatcher) })

	s.lifeGetter = mockLifeGetter{
		life: life.Alive,
	}
	s.charmGetter = mockCharmGetter{
		charmInfo: &charms.CharmInfo{
			Manifest: &charm.Manifest{},
			Meta:     &charm.Meta{},
		},
	}
	s.serviceExposer = mockServiceExposer{
		exposed:   s.serviceExposed,
		unexposed: s.serviceUnexposed,
	}

	s.config = caasfirewaller.Config{
		ControllerUUID:    coretesting.ControllerTag.Id(),
		ModelUUID:         coretesting.ModelTag.Id(),
		ApplicationGetter: &s.applicationGetter,
		ServiceExposer:    &s.serviceExposer,
		LifeGetter:        &s.lifeGetter,
		CharmGetter:       &s.charmGetter,
		Logger:            loggo.GetLogger("test"),
	}
}

func (s *WorkerSuite) sendApplicationExposedChange(c *gc.C) {
	select {
	case s.appExposedChange <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending application exposed change")
	}
}

func (s *WorkerSuite) TestValidateConfig(c *gc.C) {
	s.testValidateConfig(c, func(config *caasfirewaller.Config) {
		config.ControllerUUID = ""
	}, `missing ControllerUUID not valid`)

	s.testValidateConfig(c, func(config *caasfirewaller.Config) {
		config.ModelUUID = ""
	}, `missing ModelUUID not valid`)

	s.testValidateConfig(c, func(config *caasfirewaller.Config) {
		config.ApplicationGetter = nil
	}, `missing ApplicationGetter not valid`)

	s.testValidateConfig(c, func(config *caasfirewaller.Config) {
		config.ServiceExposer = nil
	}, `missing ServiceExposer not valid`)

	s.testValidateConfig(c, func(config *caasfirewaller.Config) {
		config.LifeGetter = nil
	}, `missing LifeGetter not valid`)

	s.testValidateConfig(c, func(config *caasfirewaller.Config) {
		config.CharmGetter = nil
	}, `missing CharmGetter not valid`)

	s.testValidateConfig(c, func(config *caasfirewaller.Config) {
		config.Logger = nil
	}, `missing Logger not valid`)
}

func (s *WorkerSuite) testValidateConfig(c *gc.C, f func(*caasfirewaller.Config), expect string) {
	config := s.config
	f(&config)
	w, err := caasfirewaller.NewWorker(config)
	if err == nil {
		workertest.DirtyKill(c, w)
	}
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	w, err := caasfirewaller.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) sendApplicationChange(c *gc.C, appName string) {
	select {
	case s.applicationChanges <- []string{appName}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}
}

func (s *WorkerSuite) TestExposedChange(c *gc.C) {
	w, err := caasfirewaller.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.sendApplicationChange(c, "gitlab")

	s.sendApplicationExposedChange(c)
	// The last known state on start up was unexposed
	// so we first call Unexpose().
	select {
	case <-s.serviceUnexposed:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be unexposed")
	}
	select {
	case <-s.serviceExposed:
		c.Fatal("service exposed unexpectedly")
	case <-time.After(coretesting.ShortWait):
	}

	s.applicationGetter.exposed = true
	s.sendApplicationExposedChange(c)
	select {
	case <-s.serviceExposed:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be exposed")
	}
	s.serviceExposer.CheckCallNames(c, "UnexposeService", "ExposeService")
	s.serviceExposer.CheckCall(c, 1, "ExposeService", "gitlab",
		map[string]string{
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
			"juju-model-uuid":      coretesting.ModelTag.Id()},
		config.ConfigAttributes{"juju-external-hostname": "exthost"})
}

func (s *WorkerSuite) TestUnexposedChange(c *gc.C) {
	w, err := caasfirewaller.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.sendApplicationChange(c, "gitlab")

	s.applicationGetter.exposed = true
	s.sendApplicationExposedChange(c)
	// The last known state on start up was exposed
	// so we first call Expose().
	select {
	case <-s.serviceExposed:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be exposed")
	}
	select {
	case <-s.serviceUnexposed:
		c.Fatal("service unexposed unexpectedly")
	case <-time.After(coretesting.ShortWait):
	}

	s.applicationGetter.exposed = false
	s.sendApplicationExposedChange(c)
	select {
	case <-s.serviceUnexposed:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be unexposed")
	}
}

func (s *WorkerSuite) TestWatchApplicationDead(c *gc.C) {
	w, err := caasfirewaller.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.lifeGetter.life = life.Dead
	s.sendApplicationChange(c, "gitlab")

	select {
	case s.appExposedChange <- struct{}{}:
		c.Fatal("unexpected watch for app exposed")
	case <-time.After(coretesting.ShortWait):
	}

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestRemoveApplicationStopsWatchingApplication(c *gc.C) {
	// Set up the errors before triggering any events to avoid racing
	// with the worker loop. First time around the loop the
	// application's alive, then it's gone.
	s.lifeGetter.SetErrors(nil, errors.NotFoundf("application"))

	w, err := caasfirewaller.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.sendApplicationChange(c, "gitlab")
	s.sendApplicationChange(c, "gitlab")

	err = workertest.CheckKilled(c, s.applicationGetter.appWatcher)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *WorkerSuite) TestRemoveApplicationStopsWorker(c *gc.C) {
	// Set up the errors before triggering any events to avoid racing
	// with the worker loop. First time around the loop the
	// application's alive, then it's gone.
	s.applicationGetter.SetErrors(nil, nil, errors.NotFoundf("application"))

	w, err := caasfirewaller.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.sendApplicationChange(c, "gitlab")

	s.applicationGetter.exposed = true
	s.sendApplicationExposedChange(c)
	select {
	case <-s.serviceExposed:
		c.Fatal("removed application should not be exposed")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *WorkerSuite) TestWatcherErrorStopsWorker(c *gc.C) {
	w, err := caasfirewaller.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.sendApplicationChange(c, "gitlab")

	s.applicationGetter.appWatcher.KillErr(errors.New("splat"))
	_ = workertest.CheckKilled(c, s.applicationGetter.appWatcher)
	_ = workertest.CheckKilled(c, s.applicationGetter.allWatcher)
	err = workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "splat")
}

func (s *WorkerSuite) TestV2CharmSkipProcessing(c *gc.C) {
	s.charmGetter.charmInfo.Manifest = &charm.Manifest{Bases: []charm.Base{{}}}
	s.charmGetter.charmInfo.Meta = &charm.Meta{}

	w, err := caasfirewaller.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.sendApplicationChange(c, "gitlab")
	s.waitCharmGetterCalls(c, "ApplicationCharmInfo")

	workertest.CleanKill(c, w)

	s.expectNoLifeGetterCalls(c)
}

func (s *WorkerSuite) TestCharmNotFound(c *gc.C) {
	w, err := caasfirewaller.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.charmGetter.charmInfo = nil

	s.sendApplicationChange(c, "gitlab")
	s.waitCharmGetterCalls(c, "ApplicationCharmInfo")

	workertest.CleanKill(c, w)

	s.expectNoLifeGetterCalls(c)
}

func (s *WorkerSuite) TestCharmChangesToV2(c *gc.C) {
	w, err := caasfirewaller.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.sendApplicationChange(c, "gitlab")
	s.waitCharmGetterCalls(c, "ApplicationCharmInfo")
	s.waitLifeGetterCalls(c, "Life")

	s.charmGetter.charmInfo.Manifest = &charm.Manifest{Bases: []charm.Base{{}}}
	s.charmGetter.charmInfo.Meta = &charm.Meta{}
	s.sendApplicationExposedChange(c)
	s.waitCharmGetterCalls(c, "ApplicationCharmInfo")

	err = workertest.CheckKilled(c, s.applicationGetter.appWatcher)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *WorkerSuite) waitCharmGetterCalls(c *gc.C, names ...string) {
	waitStubCalls(c, &s.charmGetter, names...)
}

func (s *WorkerSuite) waitLifeGetterCalls(c *gc.C, names ...string) {
	waitStubCalls(c, &s.lifeGetter, names...)
}

type waitStub interface {
	Calls() []testing.StubCall
	CheckCallNames(c *gc.C, expected ...string) bool
	ResetCalls()
}

func waitStubCalls(c *gc.C, stub waitStub, names ...string) {
	retryCallArgs := coretesting.LongRetryStrategy
	retryCallArgs.Func = func() error {
		if len(stub.Calls()) >= len(names) {
			return nil
		}
		return errors.Errorf("Not enough calls yet")
	}
	err := retry.Call(retryCallArgs)
	c.Assert(err, jc.ErrorIsNil)

	stub.CheckCallNames(c, names...)
	stub.ResetCalls()
}

func (s *WorkerSuite) expectNoLifeGetterCalls(c *gc.C) {
	totalDuration := clock.WallClock.After(coretesting.ShortWait)
	for {
		select {
		case <-clock.WallClock.After(10 * time.Millisecond):
			if len(s.lifeGetter.Calls()) > 0 {
				c.Fatalf("unexpected lifegetter call")
			}
		case <-totalDuration:
			return
		}
	}
}
