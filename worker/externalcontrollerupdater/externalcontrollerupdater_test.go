// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/crosscontroller"
	"github.com/juju/juju/core/crossmodel"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/externalcontrollerupdater"
)

var _ = gc.Suite(&ExternalControllerUpdaterSuite{})

type ExternalControllerUpdaterSuite struct {
	coretesting.BaseSuite

	updater mockExternalControllerUpdaterClient
	watcher mockExternalControllerWatcherClient

	clock *testclock.Clock

	stub       testing.Stub
	newWatcher externalcontrollerupdater.NewExternalControllerWatcherClientFunc
}

func (s *ExternalControllerUpdaterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.updater = mockExternalControllerUpdaterClient{
		watcher: newMockStringsWatcher(),
		info: crossmodel.ControllerInfo{
			ControllerTag: coretesting.ControllerTag,
			Alias:         "foo",
			Addrs:         []string{"bar"},
			CACert:        "baz",
		},
	}
	s.AddCleanup(func(*gc.C) { s.updater.watcher.Stop() })

	s.watcher = mockExternalControllerWatcherClient{
		watcher: newMockNotifyWatcher(),
		info: crosscontroller.ControllerInfo{
			Addrs:  []string{"foo"},
			CACert: "bar",
		},
	}
	s.AddCleanup(func(*gc.C) { s.watcher.watcher.Stop() })

	s.clock = testclock.NewClock(time.Time{})

	s.stub.ResetCalls()
	s.newWatcher = func(apiInfo *api.Info) (externalcontrollerupdater.ExternalControllerWatcherClientCloser, error) {
		s.stub.AddCall("NextExternalControllerWatcherClient", apiInfo)
		if err := s.stub.NextErr(); err != nil {
			return nil, err
		}
		return &s.watcher, nil
	}
}

func (s *ExternalControllerUpdaterSuite) TestStartStop(c *gc.C) {
	w, err := externalcontrollerupdater.New(&s.updater, s.newWatcher, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllersCalled(c *gc.C) {
	s.updater.watcher.changes = make(chan []string)

	w, err := externalcontrollerupdater.New(&s.updater, s.newWatcher, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case s.updater.watcher.changes <- []string{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting to send changes")
	}

	workertest.CleanKill(c, w)
	s.updater.Stub.CheckCallNames(c, "WatchExternalControllers")
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllers(c *gc.C) {
	s.updater.watcher.changes <- []string{coretesting.ControllerTag.Id()}

	w, err := externalcontrollerupdater.New(&s.updater, s.newWatcher, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// Cause three notifications. Only the first notification is
	// accompanied by API address changes, so there should be only
	// one API reconnection, and one local controller update.
	for i := 0; i < 3; i++ {
		select {
		case s.watcher.watcher.changes <- struct{}{}:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting to send changes")
		}
	}

	s.stub.CheckCalls(c, []testing.StubCall{{
		"NextExternalControllerWatcherClient",
		[]interface{}{&api.Info{
			Addrs:  s.updater.info.Addrs,
			CACert: s.updater.info.CACert,
			Tag:    names.NewUserTag("jujuanonymous"),
		}},
	}, {
		"NextExternalControllerWatcherClient",
		[]interface{}{&api.Info{
			Addrs:  s.watcher.info.Addrs,
			CACert: s.updater.info.CACert, // only addresses are updated
			Tag:    names.NewUserTag("jujuanonymous"),
		}},
	}})
	s.updater.Stub.CheckCalls(c, []testing.StubCall{{
		"WatchExternalControllers",
		[]interface{}{},
	}, {
		"ExternalControllerInfo",
		[]interface{}{coretesting.ControllerTag.Id()},
	}, {
		"SetExternalControllerInfo",
		[]interface{}{crossmodel.ControllerInfo{
			ControllerTag: s.updater.info.ControllerTag,
			Alias:         s.updater.info.Alias,
			Addrs:         s.watcher.info.Addrs, // new addrs
			CACert:        s.updater.info.CACert,
		}},
	}})
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if len(s.watcher.Stub.Calls()) < 6 {
			continue
		}
		s.watcher.Stub.CheckCallNames(c,
			"WatchControllerInfo",
			"ControllerInfo",
			"Close", // close watcher and restart when a change arrives
			"WatchControllerInfo",
			"ControllerInfo", // no change
			"ControllerInfo", // no change
		)
		return
	}
	c.Fatal("time out waiting for worker api calls")
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllersErrorsContained(c *gc.C) {
	// The first time we attempt to connect to the external controller,
	// the dial should fail. The runner will reschedule the worker to
	// try again.
	s.stub.SetErrors(errors.New("no API connection for you"))

	s.updater.watcher.changes <- []string{coretesting.ControllerTag.Id()}
	s.watcher.watcher.changes = make(chan struct{})
	s.watcher.info.Addrs = s.updater.info.Addrs // no change

	w, err := externalcontrollerupdater.New(&s.updater, s.newWatcher, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// The first run of the controller worker should fail to
	// connect to the API, and should abort. The runner should
	// then be waiting for a minute to restart the controller
	// worker.
	s.clock.WaitAdvance(time.Second, coretesting.LongWait, 1)
	s.clock.WaitAdvance(59*time.Second, coretesting.LongWait, 1)

	// The controller worker should have been restarted now.
	select {
	case s.watcher.watcher.changes <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting to send changes")
	}

	workertest.CleanKill(c, w)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"NextExternalControllerWatcherClient",
		[]interface{}{&api.Info{
			Addrs:  s.updater.info.Addrs,
			CACert: s.updater.info.CACert,
			Tag:    names.NewUserTag("jujuanonymous"),
		}},
	}, {
		"NextExternalControllerWatcherClient",
		[]interface{}{&api.Info{
			Addrs:  s.updater.info.Addrs,
			CACert: s.updater.info.CACert,
			Tag:    names.NewUserTag("jujuanonymous"),
		}},
	}})
	s.updater.Stub.CheckCallNames(c,
		"WatchExternalControllers",
		"ExternalControllerInfo",
		"ExternalControllerInfo",
	)
	s.watcher.Stub.CheckCallNames(c,
		"WatchControllerInfo",
		"ControllerInfo",
		"Close",
	)
}
