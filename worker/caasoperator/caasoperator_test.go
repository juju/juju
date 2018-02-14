// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	agenttools "github.com/juju/juju/agent/tools"
	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/downloader"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher/watchertest"
	"github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/uniter"
	runnertesting "github.com/juju/juju/worker/uniter/runner/testing"
	"github.com/juju/juju/worker/workertest"
)

type WorkerSuite struct {
	testing.IsolationSuite

	clock                 *testing.Clock
	config                caasoperator.Config
	unitsChanges          chan []string
	client                fakeClient
	charmDownloader       fakeDownloader
	charmSHA256           string
	uniterParams          *uniter.UniterParams
	leadershipTrackerFunc func(unitTag names.UnitTag) leadership.Tracker
	uniterFacadeFunc      func(unitTag names.UnitTag) *apiuniter.State
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)

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
}

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.clock = testing.NewClock(time.Time{})
	s.client = fakeClient{}
	s.unitsChanges = make(chan []string)
	s.client.unitsWatcher = watchertest.NewMockStringsWatcher(s.unitsChanges)
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
		ContainerSpecSetter:   &s.client,
		DataDir:               c.MkDir(),
		Downloader:            &s.charmDownloader,
		StatusSetter:          &s.client,
		APIAddressGetter:      &s.client,
		ProxySettingsGetter:   &s.client,
		LifeGetter:            &s.client,
		UnitGetter:            &s.client,
		UniterParams:          s.uniterParams,
		LeadershipTrackerFunc: s.leadershipTrackerFunc,
		UniterFacadeFunc:      s.uniterFacadeFunc,
		StartUniterFunc:       func(runner *worker.Runner, params *uniter.UniterParams) error { return nil },
	}

	agentBinaryDir := agenttools.ToolsDir(s.config.DataDir, "application-gitlab")
	err := os.MkdirAll(agentBinaryDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(s.config.DataDir, "tools", "jujud"), []byte("jujud"), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *WorkerSuite) TestValidateConfig(c *gc.C) {
	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.Application = ""
	}, `application name "" not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.LifeGetter = nil
	}, `missing LifeGetter not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.UnitGetter = nil
	}, `missing UnitGetter not valid`)

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
		config.ContainerSpecSetter = nil
	}, `missing ContainerSpecSetter not valid`)

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
		config.APIAddressGetter = nil
	}, `missing APIAddressGetter not valid`)
	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.ProxySettingsGetter = nil
	}, `missing ProxySettingsGetter not valid`)
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
	var uniterStarted int32
	s.config.StartUniterFunc = func(runner *worker.Runner, params *uniter.UniterParams) error {
		c.Assert(params.UnitTag.Id(), gc.Equals, "gitlab/0")
		atomic.AddInt32(&uniterStarted, 1)
		return nil
	}

	w, err := caasoperator.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.unitsChanges <- []string{"gitlab/0"}
	s.client.CheckCallNames(c, "Charm", "SetStatus", "WatchUnits", "SetStatus", "Life")
	s.client.CheckCall(c, 0, "Charm", "gitlab")
	s.client.CheckCall(c, 2, "WatchUnits", "gitlab")
	s.client.CheckCall(c, 4, "Life", "gitlab/0")

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
		TargetDir: filepath.Join(agentDir, "charm.dl"),
	})

	// The download directory should have been removed.
	_, err = os.Stat(downloadRequest.TargetDir)
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// The charm archive should have been unpacked into <data-dir>/charm.
	charmDir := filepath.Join(agentDir, "charm")
	_, err = os.Stat(filepath.Join(charmDir, "metadata.yaml"))
	c.Assert(err, jc.ErrorIsNil)

	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if atomic.LoadInt32(&uniterStarted) > 0 {
			return
		}
	}
	c.Fatalf("timeout while waiting for uniter to start")
}

func (s *WorkerSuite) TestWorkerSetsStatus(c *gc.C) {
	w, err := caasoperator.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)

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
