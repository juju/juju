// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/storageprovisioner"
)

type WorkerSuite struct {
	testing.IsolationSuite

	config              storageprovisioner.Config
	applicationsWatcher *mockApplicationsWatcher
	lifeGetter          *mockLifecycleManager

	applicationChanges chan []string

	clock *testclock.Clock
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.applicationChanges = make(chan []string)
	s.applicationsWatcher = newMockApplicationsWatcher(s.applicationChanges)
	s.lifeGetter = &mockLifecycleManager{}

	s.config = storageprovisioner.Config{
		Model:            coretesting.ModelTag,
		Scope:            coretesting.ModelTag,
		Applications:     s.applicationsWatcher,
		Volumes:          newMockVolumeAccessor(),
		Filesystems:      newMockFilesystemAccessor(),
		Life:             s.lifeGetter,
		Status:           &mockStatusSetter{},
		Clock:            &mockClock{},
		Logger:           loggo.GetLogger("test"),
		Registry:         storage.StaticProviderRegistry{},
		CloudCallContext: context.NewCloudCallContext(),
	}
}

func (s *WorkerSuite) TestValidateConfig(c *gc.C) {
	s.testValidateConfig(c, func(config *storageprovisioner.Config) {
		config.Scope = names.NewApplicationTag("mariadb")
		config.Applications = nil
	}, `nil Applications not valid`)
}

func (s *WorkerSuite) testValidateConfig(c *gc.C, f func(*storageprovisioner.Config), expect string) {
	config := s.config
	f(&config)
	w, err := storageprovisioner.NewCaasWorker(config)
	if err == nil {
		workertest.DirtyKill(c, w)
	}
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	w, err := storageprovisioner.NewCaasWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) setupNewUnitScenario(c *gc.C) worker.Worker {
	w, err := storageprovisioner.NewCaasWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case s.applicationChanges <- []string{"mariadb"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}
	return w
}

func (s *WorkerSuite) TestWatchApplicationDead(c *gc.C) {
	w, err := storageprovisioner.NewCaasWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case s.applicationChanges <- []string{"postgresql"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	// Given the worker time to startup.
	shortAttempt := &utils.AttemptStrategy{
		Total: coretesting.LongWait,
		Delay: 10 * time.Millisecond,
	}
	for a := shortAttempt.Start(); a.Next(); {
		if len(s.config.Filesystems.(*mockFilesystemAccessor).Calls()) > 0 {
			break
		}
	}

	workertest.CleanKill(c, w)
	// Only call is to watch model.
	s.config.Filesystems.(*mockFilesystemAccessor).CheckCallNames(c, "WatchFilesystems")
	s.config.Filesystems.(*mockFilesystemAccessor).CheckCall(c, 0, "WatchFilesystems", coretesting.ModelTag)
}

func (s *WorkerSuite) TestRemoveApplicationStopsWatchingApplication(c *gc.C) {
	w, err := storageprovisioner.NewCaasWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case s.applicationChanges <- []string{"mariadb"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	// Check that the worker is running or not;
	// given it time to startup.
	shortAttempt := &utils.AttemptStrategy{
		Total: coretesting.LongWait,
		Delay: 10 * time.Millisecond,
	}
	running := false
	for a := shortAttempt.Start(); a.Next(); {
		_, running = storageprovisioner.StorageWorker(w, "mariadb")
		if running {
			break
		}
	}
	c.Assert(running, jc.IsTrue)

	// Add an additional app worker so we can check that the correct one is accessed.
	storageprovisioner.NewStorageWorker(w, "postgresql")

	s.lifeGetter.err = &params.Error{Code: params.CodeNotFound}
	select {
	case s.applicationChanges <- []string{"postgresql"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	// The mariadb worker should still be running.
	_, ok := storageprovisioner.StorageWorker(w, "mariadb")
	c.Assert(ok, jc.IsTrue)

	// Check that the postgresql worker is running or not;
	// given it time to shutdown.
	running = true
	for a := shortAttempt.Start(); a.Next(); {
		_, running = storageprovisioner.StorageWorker(w, "postgresql")
		if !running {
			break
		}
	}
	c.Assert(running, jc.IsFalse)
	workertest.CleanKill(c, w)
	workertest.CheckKilled(c, s.applicationsWatcher.watcher)
}

func (s *WorkerSuite) TestWatcherErrorStopsWorker(c *gc.C) {
	w, err := storageprovisioner.NewCaasWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case s.applicationChanges <- []string{"mariadb"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	s.applicationsWatcher.watcher.KillErr(errors.New("splat"))
	workertest.CheckKilled(c, s.applicationsWatcher.watcher)
	err = workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "splat")
}
