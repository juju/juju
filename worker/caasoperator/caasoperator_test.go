// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/os/series"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"

	agenttools "github.com/juju/juju/agent/tools"
	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/downloader"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/uniter"
	runnertesting "github.com/juju/juju/worker/uniter/runner/testing"
)

type WorkerSuite struct {
	testing.IsolationSuite

	clock                 *testclock.Clock
	config                caasoperator.Config
	unitsChanges          chan []string
	appChanges            chan struct{}
	appWatched            chan struct{}
	unitRemoved           chan struct{}
	client                fakeClient
	charmDownloader       fakeDownloader
	charmSHA256           string
	uniterParams          *uniter.UniterParams
	leadershipTrackerFunc func(unitTag names.UnitTag) leadership.Tracker
	uniterFacadeFunc      func(unitTag names.UnitTag) *apiuniter.State
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	// Create a charm archive, and compute its SHA256 hash
	// for comparison in the tests.
	fakeDownloadDir := c.MkDir()
	s.charmDownloader = fakeDownloader{
		path: testcharms.Repo.CharmArchivePath(
			fakeDownloadDir,
			"../kubernetes/gitlab",
		),
	}
	charmSHA256, _, err := utils.ReadFileSHA256(s.charmDownloader.path)
	c.Assert(err, jc.ErrorIsNil)
	s.charmSHA256 = charmSHA256

	s.clock = testclock.NewClock(time.Time{})
	s.appWatched = make(chan struct{}, 1)
	s.unitRemoved = make(chan struct{}, 1)
	s.client = fakeClient{
		applicationWatched: s.appWatched,
		unitRemoved:        s.unitRemoved,
		life:               life.Alive,
	}
	s.unitsChanges = make(chan []string)
	s.appChanges = make(chan struct{})
	s.client.unitsWatcher = watchertest.NewMockStringsWatcher(s.unitsChanges)
	s.client.watcher = watchertest.NewMockNotifyWatcher(s.appChanges)
	s.charmDownloader.ResetCalls()
	s.uniterParams = &uniter.UniterParams{}
	s.leadershipTrackerFunc = func(unitTag names.UnitTag) leadership.Tracker {
		return &runnertesting.FakeTracker{}
	}
	s.uniterFacadeFunc = func(unitTag names.UnitTag) *apiuniter.State {
		return &apiuniter.State{}
	}
	s.config = caasoperator.Config{
		Application:           "gitlab",
		CharmGetter:           &s.client,
		Clock:                 s.clock,
		PodSpecSetter:         &s.client,
		DataDir:               c.MkDir(),
		Downloader:            &s.charmDownloader,
		StatusSetter:          &s.client,
		ApplicationWatcher:    &s.client,
		UnitGetter:            &s.client,
		UnitRemover:           &s.client,
		VersionSetter:         &s.client,
		UniterParams:          s.uniterParams,
		LeadershipTrackerFunc: s.leadershipTrackerFunc,
		UniterFacadeFunc:      s.uniterFacadeFunc,
		StartUniterFunc:       func(runner *worker.Runner, params *uniter.UniterParams) error { return nil },
	}

	agentBinaryDir := agenttools.ToolsDir(s.config.DataDir, "application-gitlab")
	err = os.MkdirAll(agentBinaryDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(s.config.DataDir, "tools", "jujud"), []byte("jujud"), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *WorkerSuite) TestValidateConfig(c *gc.C) {
	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.Application = ""
	}, `application name "" not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.ApplicationWatcher = nil
	}, `missing ApplicationWatcher not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.UnitGetter = nil
	}, `missing UnitGetter not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.UnitRemover = nil
	}, `missing UnitRemover not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.LeadershipTrackerFunc = nil
	}, `missing LeadershipTrackerFunc not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.UniterFacadeFunc = nil
	}, `missing UniterFacadeFunc not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.UniterParams = nil
	}, `missing UniterParams not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.CharmGetter = nil
	}, `missing CharmGetter not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.Clock = nil
	}, `missing Clock not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.PodSpecSetter = nil
	}, `missing PodSpecSetter not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.DataDir = ""
	}, `missing DataDir not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.Downloader = nil
	}, `missing Downloader not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.StatusSetter = nil
	}, `missing StatusSetter not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.VersionSetter = nil
	}, `missing VersionSetter not valid`)

}

func (s *WorkerSuite) testValidateConfig(c *gc.C, f func(*caasoperator.Config), expect string) {
	config := s.config
	f(&config)
	w, err := caasoperator.NewWorker(config)
	if err == nil {
		workertest.DirtyKill(c, w)
	}
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	w, err := caasoperator.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerDownloadsCharm(c *gc.C) {
	uniterStarted := make(chan struct{})
	s.config.StartUniterFunc = func(runner *worker.Runner, params *uniter.UniterParams) error {
		c.Assert(params.UnitTag.Id(), gc.Equals, "gitlab/0")
		close(uniterStarted)
		return nil
	}

	w, err := caasoperator.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case s.appChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending application change")
	}
	select {
	case s.unitsChanges <- []string{"gitlab/0"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending unit change")
	}
	select {
	case <-s.appWatched:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for application to be watched")
	}
	select {
	case <-uniterStarted:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for uniter to start")
	}

	s.client.CheckCallNames(c, "Charm", "SetStatus", "SetVersion", "WatchUnits", "SetStatus", "Watch", "Charm", "Life")
	s.client.CheckCall(c, 0, "Charm", "gitlab")
	s.client.CheckCall(c, 2, "SetVersion", "gitlab", version.Binary{
		Number: jujuversion.Current,
		Series: series.MustHostSeries(),
		Arch:   arch.HostArch(),
	})
	s.client.CheckCall(c, 3, "WatchUnits", "gitlab")
	s.client.CheckCall(c, 5, "Watch", "gitlab")

	s.charmDownloader.CheckCallNames(c, "Download")
	downloadArgs := s.charmDownloader.Calls()[0].Args
	c.Assert(downloadArgs, gc.HasLen, 1)
	c.Assert(downloadArgs[0], gc.FitsTypeOf, downloader.Request{})
	downloadRequest := downloadArgs[0].(downloader.Request)
	c.Assert(downloadRequest.Abort, gc.NotNil)
	c.Assert(downloadRequest.Verify, gc.NotNil)

	// fakeClient.Charm returns the SHA256 sum of fakeCharmContent.
	fakeCharmPath := filepath.Join(c.MkDir(), "fake.charm")
	err = ioutil.WriteFile(fakeCharmPath, fakeCharmContent, 0644)
	c.Assert(err, jc.ErrorIsNil)
	f, err := os.Open(fakeCharmPath)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	err = downloadRequest.Verify(f)
	c.Assert(err, jc.ErrorIsNil)

	downloadRequest.Abort = nil
	downloadRequest.Verify = nil
	agentDir := filepath.Join(s.config.DataDir, "agents", "application-gitlab")
	c.Assert(downloadRequest, jc.DeepEquals, downloader.Request{
		URL:       &url.URL{Scheme: "cs", Opaque: "gitlab-1"},
		TargetDir: filepath.Join(agentDir, "state", "bundles", "downloads"),
	})

	// The download directory should have been removed.
	_, err = os.Stat(downloadRequest.TargetDir)
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// The charm archive should have been unpacked into <data-dir>/charm.
	charmDir := filepath.Join(agentDir, "charm")
	_, err = os.Stat(filepath.Join(charmDir, "metadata.yaml"))
	c.Assert(err, jc.ErrorIsNil)

}

func (s *WorkerSuite) assertUniterStarted(c *gc.C) (worker.Worker, watcher.NotifyChannel) {
	applicationChannel := make(chan watcher.NotifyChannel)
	s.config.StartUniterFunc = func(runner *worker.Runner, params *uniter.UniterParams) error {
		c.Assert(params.UnitTag.Id(), gc.Equals, "gitlab/0")
		select {
		case applicationChannel <- params.ApplicationChannel:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timeout sending application channel")
		}
		return nil
	}

	w, err := caasoperator.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case s.appChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending application change")
	}
	select {
	case s.unitsChanges <- []string{"gitlab/0"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending unit change")
	}

	select {
	case channel := <-applicationChannel:
		return w, channel
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for uniter to start")
	}
	panic("not reachable")
}

func (s *WorkerSuite) TestUpgradeCharm(c *gc.C) {
	w, applicationChannel := s.assertUniterStarted(c)
	defer workertest.CleanKill(c, w)

	select {
	case <-applicationChannel:
		c.Fatal("unexpected application change")
	case <-time.After(coretesting.ShortWait):
	}

	select {
	case s.appChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending application change")
	}

	select {
	case <-applicationChannel:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for application change")
	}
}

func (s *WorkerSuite) TestWorkerSetsStatus(c *gc.C) {
	w, err := caasoperator.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)

	s.client.CheckCallNames(c, "Charm", "SetStatus", "SetVersion", "WatchUnits", "SetStatus", "Watch")
	s.client.CheckCall(c, 1, "SetStatus", "gitlab", status.Maintenance, "downloading charm (cs:gitlab-1)", map[string]interface{}(nil))
}

func (s *WorkerSuite) TestWatcherFailureStopsWorker(c *gc.C) {
	w, err := caasoperator.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.client.unitsWatcher.KillErr(errors.New("splat"))
	err = workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "splat")
}

func (s *WorkerSuite) TestRemovedUnit(c *gc.C) {
	w, _ := s.assertUniterStarted(c)
	defer workertest.CleanKill(c, w)

	s.client.ResetCalls()
	s.client.life = life.Dead
	select {
	case s.unitsChanges <- []string{"gitlab/0"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending unit change")
	}
	select {
	case <-s.unitRemoved:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for unit to be removed")
	}
	s.client.CheckCallNames(c, "Life", "RemoveUnit")
	s.client.CheckCall(c, 0, "Life", "gitlab/0")
	s.client.CheckCall(c, 1, "RemoveUnit", "gitlab/0")
}
