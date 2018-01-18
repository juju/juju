// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher/watchertest"
	"github.com/juju/juju/worker/caasunitprovisioner"
	"github.com/juju/juju/worker/workertest"
)

type WorkerSuite struct {
	testing.IsolationSuite

	config              caasunitprovisioner.Config
	applicationGetter   mockApplicationGetter
	serviceBroker       mockServiceBroker
	containerBroker     mockContainerBroker
	containerSpecGetter mockContainerSpecGetter
	lifeGetter          mockLifeGetter
	unitGetter          mockUnitGetter
	unitUpdater         mockUnitUpdater

	applicationChanges   chan []string
	jujuUnitChanges      chan []string
	caasUnitsChanges     chan struct{}
	containerSpecChanges chan struct{}
	serviceEnsured       chan struct{}
	unitEnsured          chan struct{}
	clock                *testing.Clock
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.applicationChanges = make(chan []string)
	s.jujuUnitChanges = make(chan []string)
	s.caasUnitsChanges = make(chan struct{})
	s.containerSpecChanges = make(chan struct{})
	s.serviceEnsured = make(chan struct{})
	s.unitEnsured = make(chan struct{})

	s.applicationGetter = mockApplicationGetter{
		watcher: watchertest.NewMockStringsWatcher(s.applicationChanges),
	}
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.applicationGetter.watcher) })

	s.containerSpecGetter = mockContainerSpecGetter{
		watcher: watchertest.NewMockNotifyWatcher(s.containerSpecChanges),
	}
	s.containerSpecGetter.setSpec("container-spec")
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.containerSpecGetter.watcher) })

	s.unitGetter = mockUnitGetter{
		watcher: watchertest.NewMockStringsWatcher(s.jujuUnitChanges),
	}
	s.unitUpdater = mockUnitUpdater{}
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.unitGetter.watcher) })

	s.containerBroker = mockContainerBroker{
		ensured:      s.unitEnsured,
		unitsWatcher: watchertest.NewMockNotifyWatcher(s.caasUnitsChanges),
	}
	s.lifeGetter = mockLifeGetter{}
	s.lifeGetter.setLife(life.Alive)
	s.serviceBroker = mockServiceBroker{
		ensured: s.serviceEnsured,
	}

	s.config = caasunitprovisioner.Config{
		ApplicationGetter:   &s.applicationGetter,
		ServiceBroker:       &s.serviceBroker,
		ContainerBroker:     &s.containerBroker,
		ContainerSpecGetter: &s.containerSpecGetter,
		LifeGetter:          &s.lifeGetter,
		UnitGetter:          &s.unitGetter,
		UnitUpdater:         &s.unitUpdater,
	}
}

func (s *WorkerSuite) sendContainerSpecChange(c *gc.C) {
	select {
	case s.containerSpecChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending container spec change")
	}
}

func (s *WorkerSuite) TestValidateConfig(c *gc.C) {
	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ApplicationGetter = nil
	}, `missing ApplicationGetter not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ServiceBroker = nil
	}, `missing ServiceBroker not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ContainerBroker = nil
	}, `missing ContainerBroker not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ContainerSpecGetter = nil
	}, `missing ContainerSpecGetter not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.LifeGetter = nil
	}, `missing LifeGetter not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.UnitGetter = nil
	}, `missing UnitGetter not valid`)
}

func (s *WorkerSuite) testValidateConfig(c *gc.C, f func(*caasunitprovisioner.Config), expect string) {
	config := s.config
	f(&config)
	w, err := caasunitprovisioner.NewWorker(config)
	if err == nil {
		workertest.DirtyKill(c, w)
	}
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) setupNewUnitScenario(c *gc.C, brokerManaged bool, opChan chan struct{}) worker.Worker {
	s.config.BrokerManagedUnits = brokerManaged
	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.containerSpecGetter.SetErrors(nil, errors.NotFoundf("spec"))

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	select {
	case s.jujuUnitChanges <- []string{"gitlab/0"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	// We seed a "not found" error above to indicate that
	// there is not yet a container spec; the broker should
	// not be invoked.
	s.sendContainerSpecChange(c)
	select {
	case <-opChan:
		c.Fatal("service/unit ensured unexpectedly")
	case <-time.After(coretesting.ShortWait):
	}

	s.sendContainerSpecChange(c)
	s.containerSpecGetter.assertSpecRetrieved(c)
	select {
	case <-opChan:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service/unit to be ensured")
	}
	return w
}

func (s *WorkerSuite) TestNewJujuManagedUnit(c *gc.C) {
	w := s.setupNewUnitScenario(c, false, s.unitEnsured)
	defer workertest.CleanKill(c, w)

	s.applicationGetter.CheckCallNames(c, "WatchApplications")
	s.unitGetter.CheckCallNames(c, "WatchUnits")
	s.unitGetter.CheckCall(c, 0, "WatchUnits", "gitlab")
	s.containerSpecGetter.CheckCallNames(c, "WatchContainerSpec", "ContainerSpec", "ContainerSpec")
	s.containerSpecGetter.CheckCall(c, 0, "WatchContainerSpec", "gitlab/0")
	s.containerSpecGetter.CheckCall(c, 1, "ContainerSpec", "gitlab/0") // not found
	s.containerSpecGetter.CheckCall(c, 2, "ContainerSpec", "gitlab/0")
	s.lifeGetter.CheckCallNames(c, "Life", "Life")
	s.lifeGetter.CheckCall(c, 0, "Life", "gitlab")
	s.lifeGetter.CheckCall(c, 1, "Life", "gitlab/0")
	s.containerBroker.CheckCallNames(c, "WatchUnits", "EnsureUnit")
	s.containerBroker.CheckCall(c, 1, "EnsureUnit", "gitlab", "gitlab/0", "container-spec")
}

func (s *WorkerSuite) TestNewBrokerManagedUnit(c *gc.C) {
	w := s.setupNewUnitScenario(c, true, s.serviceEnsured)
	defer workertest.CleanKill(c, w)

	s.applicationGetter.CheckCallNames(c, "WatchApplications", "ApplicationConfig")
	s.containerSpecGetter.CheckCallNames(c, "WatchContainerSpec", "ContainerSpec", "ContainerSpec")
	s.containerSpecGetter.CheckCall(c, 0, "WatchContainerSpec", "gitlab/0")
	s.containerSpecGetter.CheckCall(c, 1, "ContainerSpec", "gitlab/0") // not found
	s.containerSpecGetter.CheckCall(c, 2, "ContainerSpec", "gitlab/0")
	s.lifeGetter.CheckCallNames(c, "Life", "Life")
	s.lifeGetter.CheckCall(c, 0, "Life", "gitlab")
	s.lifeGetter.CheckCall(c, 1, "Life", "gitlab/0")
	s.serviceBroker.CheckCallNames(c, "EnsureService")
	s.serviceBroker.CheckCall(c, 0, "EnsureService",
		"gitlab", "container-spec", 1, application.ConfigAttributes{"juju-external-hostname": "exthost"})

	s.serviceBroker.ResetCalls()
	// Add another unit.
	select {
	case s.jujuUnitChanges <- []string{"gitlab/1"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}

	s.serviceBroker.CheckCallNames(c, "EnsureService")
	s.serviceBroker.CheckCall(c, 0, "EnsureService",
		"gitlab", "container-spec", 2, application.ConfigAttributes{"juju-external-hostname": "exthost"})

	s.serviceBroker.ResetCalls()
	// Delete a unit.
	s.lifeGetter.setLife(life.Dead)
	select {
	case s.jujuUnitChanges <- []string{"gitlab/0"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}

	s.serviceBroker.CheckCallNames(c, "EnsureService")
	s.serviceBroker.CheckCall(c, 0, "EnsureService",
		"gitlab", "container-spec", 1, application.ConfigAttributes{"juju-external-hostname": "exthost"})
}

func (s *WorkerSuite) TestNewBrokerManagedUnitSpecChange(c *gc.C) {
	w := s.setupNewUnitScenario(c, true, s.serviceEnsured)
	defer workertest.CleanKill(c, w)

	s.serviceBroker.ResetCalls()

	// Same spec, nothing happens.
	s.sendContainerSpecChange(c)
	s.containerSpecGetter.assertSpecRetrieved(c)
	select {
	case <-s.serviceEnsured:
		c.Fatal("service/unit ensured unexpectedly")
	case <-time.After(coretesting.ShortWait):
	}

	s.containerSpecGetter.setSpec("another-spec")
	s.sendContainerSpecChange(c)
	s.containerSpecGetter.assertSpecRetrieved(c)

	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}

	s.serviceBroker.CheckCallNames(c, "EnsureService")
	s.serviceBroker.CheckCall(c, 0, "EnsureService",
		"gitlab", "another-spec", 1, application.ConfigAttributes{"juju-external-hostname": "exthost"})
}

func (s *WorkerSuite) TestNewBrokerManagedUnitAllRemoved(c *gc.C) {
	w := s.setupNewUnitScenario(c, true, s.serviceEnsured)
	defer workertest.CleanKill(c, w)

	s.serviceBroker.ResetCalls()
	// Add another unit.
	select {
	case s.jujuUnitChanges <- []string{"gitlab/1"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}
	s.serviceBroker.ResetCalls()

	// Now the units die.
	s.lifeGetter.setLife(life.Dead)
	select {
	case s.jujuUnitChanges <- []string{"gitlab/0", "gitlab/1"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	select {
	case <-s.serviceEnsured:
		c.Fatal("service/unit ensured unexpectedly")
	case <-time.After(coretesting.ShortWait):
	}

	s.serviceBroker.CheckCallNames(c, "DeleteService")
	s.serviceBroker.CheckCall(c, 0, "DeleteService", "gitlab")
}

func (s *WorkerSuite) TestWatchApplicationDead(c *gc.C) {
	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.lifeGetter.setLife(life.Dead)
	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	select {
	case s.jujuUnitChanges <- []string{"gitlab/0"}:
		c.Fatal("unexpected watch for units")
	case <-time.After(coretesting.ShortWait):
	}

	workertest.CleanKill(c, w)
	s.unitGetter.CheckNoCalls(c)
}

func (s *WorkerSuite) TestWatchUnitDead(c *gc.C) {
	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}
	// application is initially alive
	s.lifeGetter.assertLifeRetrieved(c)

	// unit is initially dead
	s.lifeGetter.setLife(life.Dead)
	select {
	case s.jujuUnitChanges <- []string{"gitlab/0"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	workertest.CleanKill(c, w)
	s.containerSpecGetter.CheckNoCalls(c)
}

func (s *WorkerSuite) TestRemoveApplicationStopsWatchingApplication(c *gc.C) {
	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	s.lifeGetter.SetErrors(errors.NotFoundf("application"))
	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	workertest.CheckKilled(c, s.unitGetter.watcher)
}

func (s *WorkerSuite) TestRemoveUnitStopsWatchingContainerSpec(c *gc.C) {
	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	select {
	case s.jujuUnitChanges <- []string{"gitlab/0"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	s.lifeGetter.SetErrors(errors.NotFoundf("unit"))
	select {
	case s.jujuUnitChanges <- []string{"gitlab/0"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	workertest.CheckKilled(c, s.containerSpecGetter.watcher)
}

func (s *WorkerSuite) TestWatcherErrorStopsWorker(c *gc.C) {
	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	select {
	case s.jujuUnitChanges <- []string{"gitlab/0"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	s.containerSpecGetter.watcher.KillErr(errors.New("splat"))
	workertest.CheckKilled(c, s.containerSpecGetter.watcher)
	workertest.CheckKilled(c, s.unitGetter.watcher)
	workertest.CheckKilled(c, s.applicationGetter.watcher)
	err = workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "splat")
}

func (s *WorkerSuite) TestUnitsChange(c *gc.C) {
	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}
	defer workertest.CleanKill(c, w)

	s.containerBroker.ResetCalls()

	select {
	case s.caasUnitsChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.containerBroker.Calls()) > 1 {
			break
		}
	}
	s.containerBroker.CheckCallNames(c, "WatchUnits", "Units")
	c.Assert(s.containerBroker.Calls()[1].Args, jc.DeepEquals, []interface{}{"gitlab"})

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.unitUpdater.Calls()) > 0 {
			break
		}
	}
	s.unitUpdater.CheckCallNames(c, "UpdateUnits")
	c.Assert(s.unitUpdater.Calls()[0].Args, jc.DeepEquals, []interface{}{
		params.UpdateApplicationUnits{
			ApplicationTag: names.NewApplicationTag("gitlab").String(),
			Units: []params.ApplicationUnitParams{
				{Id: "u1", Address: "10.0.0.1", Ports: []string(nil), Status: "allocating"},
			},
		},
	})
}
