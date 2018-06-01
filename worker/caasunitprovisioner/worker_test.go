// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher/watchertest"
	"github.com/juju/juju/worker/caasunitprovisioner"
	"github.com/juju/juju/worker/workertest"
)

type WorkerSuite struct {
	testing.IsolationSuite

	config             caasunitprovisioner.Config
	applicationGetter  mockApplicationGetter
	applicationUpdater mockApplicationUpdater
	serviceBroker      mockServiceBroker
	containerBroker    mockContainerBroker
	podSpecGetter      mockPodSpecGetter
	lifeGetter         mockLifeGetter
	unitGetter         mockUnitGetter
	unitUpdater        mockUnitUpdater

	applicationChanges   chan []string
	jujuUnitChanges      chan []string
	caasUnitsChanges     chan struct{}
	containerSpecChanges chan struct{}
	serviceDeleted       chan struct{}
	serviceEnsured       chan struct{}
	serviceUpdated       chan struct{}
	unitEnsured          chan struct{}
	unitDeleted          chan struct{}
	clock                *testing.Clock
}

var _ = gc.Suite(&WorkerSuite{})

var (
	containerSpec = `
containers:
  - name: gitlab
    image: gitlab/latest
    ports:
    - containerPort: 80
      protocol: TCP
    - containerPort: 443
    config:
      attr: foo=bar; fred=blogs
      foo: bar
`[1:]

	parsedSpec = caas.PodSpec{
		Containers: []caas.ContainerSpec{{
			Name:  "gitlab",
			Image: "gitlab/latest",
			Ports: []caas.ContainerPort{
				{ContainerPort: 80, Protocol: "TCP"},
				{ContainerPort: 443},
			},
			Config: map[string]string{
				"attr": "foo=bar; fred=blogs",
				"foo":  "bar",
			}},
		}}
)

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.applicationChanges = make(chan []string)
	s.jujuUnitChanges = make(chan []string)
	s.caasUnitsChanges = make(chan struct{})
	s.containerSpecChanges = make(chan struct{}, 1)
	s.serviceDeleted = make(chan struct{})
	s.serviceEnsured = make(chan struct{})
	s.serviceUpdated = make(chan struct{})
	s.unitEnsured = make(chan struct{})
	s.unitDeleted = make(chan struct{})

	s.applicationGetter = mockApplicationGetter{
		watcher: watchertest.NewMockStringsWatcher(s.applicationChanges),
	}
	s.applicationUpdater = mockApplicationUpdater{
		updated: s.serviceUpdated,
	}

	s.podSpecGetter = mockPodSpecGetter{
		watcher: watchertest.NewMockNotifyWatcher(s.containerSpecChanges),
	}
	s.podSpecGetter.setSpec(containerSpec)

	s.unitGetter = mockUnitGetter{
		watcher: watchertest.NewMockStringsWatcher(s.jujuUnitChanges),
	}
	s.unitUpdater = mockUnitUpdater{}

	s.containerBroker = mockContainerBroker{
		serviceDeleted: s.serviceDeleted,
		ensured:        s.unitEnsured,
		unitDeleted:    s.unitDeleted,
		unitsWatcher:   watchertest.NewMockNotifyWatcher(s.caasUnitsChanges),
		podSpec:        &parsedSpec,
	}
	s.lifeGetter = mockLifeGetter{}
	s.lifeGetter.setLife(life.Alive)
	s.serviceBroker = mockServiceBroker{
		ensured: s.serviceEnsured,
		podSpec: &parsedSpec,
	}

	s.config = caasunitprovisioner.Config{
		ApplicationGetter:  &s.applicationGetter,
		ApplicationUpdater: &s.applicationUpdater,
		ServiceBroker:      &s.serviceBroker,
		ContainerBroker:    &s.containerBroker,
		PodSpecGetter:      &s.podSpecGetter,
		LifeGetter:         &s.lifeGetter,
		UnitGetter:         &s.unitGetter,
		UnitUpdater:        &s.unitUpdater,
	}
}

func (s *WorkerSuite) sendContainerSpecChange(c *gc.C) {
	select {
	case s.containerSpecChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending pod spec change")
	}
}

func (s *WorkerSuite) TestValidateConfig(c *gc.C) {
	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ApplicationGetter = nil
	}, `missing ApplicationGetter not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ApplicationUpdater = nil
	}, `missing ApplicationUpdater not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ServiceBroker = nil
	}, `missing ServiceBroker not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ContainerBroker = nil
	}, `missing ContainerBroker not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.PodSpecGetter = nil
	}, `missing PodSpecGetter not valid`)

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

func (s *WorkerSuite) setupNewUnitScenario(c *gc.C) worker.Worker {
	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.podSpecGetter.SetErrors(nil, errors.NotFoundf("spec"))

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
	// there is not yet a pod spec; the broker should
	// not be invoked.
	s.sendContainerSpecChange(c)
	select {
	case <-s.serviceEnsured:
		c.Fatal("service ensured unexpectedly")
	case <-time.After(coretesting.ShortWait):
	}

	s.sendContainerSpecChange(c)
	s.podSpecGetter.assertSpecRetrieved(c)
	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}
	select {
	case <-s.serviceUpdated:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be updated")
	}
	return w
}

func (s *WorkerSuite) TestUnitChanged(c *gc.C) {
	w := s.setupNewUnitScenario(c)
	defer workertest.CleanKill(c, w)

	s.applicationGetter.CheckCallNames(c, "WatchApplications", "ApplicationConfig")
	s.podSpecGetter.CheckCallNames(c, "WatchPodSpec", "PodSpec", "PodSpec")
	s.podSpecGetter.CheckCall(c, 0, "WatchPodSpec", "gitlab")
	s.podSpecGetter.CheckCall(c, 1, "PodSpec", "gitlab") // not found
	s.podSpecGetter.CheckCall(c, 2, "PodSpec", "gitlab")
	s.lifeGetter.CheckCallNames(c, "Life", "Life")
	s.lifeGetter.CheckCall(c, 0, "Life", "gitlab")
	s.lifeGetter.CheckCall(c, 1, "Life", "gitlab/0")
	s.serviceBroker.CheckCallNames(c, "EnsureService", "Service")
	s.serviceBroker.CheckCall(c, 0, "EnsureService",
		"gitlab", &parsedSpec, 1, application.ConfigAttributes{"juju-external-hostname": "exthost"})
	s.serviceBroker.CheckCall(c, 1, "Service", "gitlab")

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
		"gitlab", &parsedSpec, 2, application.ConfigAttributes{"juju-external-hostname": "exthost"})

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
		"gitlab", &parsedSpec, 1, application.ConfigAttributes{"juju-external-hostname": "exthost"})
}

func (s *WorkerSuite) TestNewPodSpecChange(c *gc.C) {
	w := s.setupNewUnitScenario(c)
	defer workertest.CleanKill(c, w)

	s.serviceBroker.ResetCalls()

	// Same spec, nothing happens.
	s.sendContainerSpecChange(c)
	s.podSpecGetter.assertSpecRetrieved(c)
	select {
	case <-s.serviceEnsured:
		c.Fatal("service/unit ensured unexpectedly")
	case <-time.After(coretesting.ShortWait):
	}

	var (
		anotherSpec = `
containers:
  - name: gitlab
    image-name: gitlab/latest
`[1:]

		anotherParsedSpec = caas.PodSpec{
			Containers: []caas.ContainerSpec{{
				Name:  "gitlab",
				Image: "gitlab/latest",
			}}}
	)

	s.serviceBroker.podSpec = &anotherParsedSpec

	s.podSpecGetter.setSpec(anotherSpec)
	s.sendContainerSpecChange(c)
	s.podSpecGetter.assertSpecRetrieved(c)

	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}

	s.serviceBroker.CheckCallNames(c, "EnsureService")
	s.serviceBroker.CheckCall(c, 0, "EnsureService",
		"gitlab", &anotherParsedSpec, 1, application.ConfigAttributes{"juju-external-hostname": "exthost"})
}

func (s *WorkerSuite) TestUnitAllRemoved(c *gc.C) {
	w := s.setupNewUnitScenario(c)
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
}

func (s *WorkerSuite) TestApplicationDeadRemovesService(c *gc.C) {
	w := s.setupNewUnitScenario(c)
	defer workertest.CleanKill(c, w)

	s.serviceBroker.ResetCalls()
	s.containerBroker.ResetCalls()

	s.lifeGetter.SetErrors(errors.NotFoundf("application"))
	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending application change")
	}

	select {
	case <-s.serviceDeleted:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be deleted")
	}

	s.containerBroker.CheckCallNames(c, "UnexposeService", "DeleteService")
	s.containerBroker.CheckCall(c, 0, "UnexposeService", "gitlab")
	s.containerBroker.CheckCall(c, 1, "DeleteService", "gitlab")
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

func (s *WorkerSuite) TestRemoveApplicationStopsWatchingApplication(c *gc.C) {
	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	// Check that the gitlab worker is running or not;
	// given it time to startup.
	shortAttempt := &utils.AttemptStrategy{
		Total: coretesting.LongWait,
		Delay: 10 * time.Millisecond,
	}
	running := false
	for a := shortAttempt.Start(); a.Next(); {
		_, running = caasunitprovisioner.AppWorker(w, "gitlab")
		if running {
			break
		}
	}
	c.Assert(running, jc.IsTrue)

	// Add an additional app worker so we can check that the correct one is accessed.
	caasunitprovisioner.NewAppWorker(w, "mysql")

	s.lifeGetter.SetErrors(errors.NotFoundf("application"))
	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	select {
	case <-s.serviceDeleted:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be deleted")
	}

	// The mysql worker should still be running.
	_, ok := caasunitprovisioner.AppWorker(w, "mysql")
	c.Assert(ok, jc.IsTrue)

	// Check that the gitlab worker is running or not;
	// given it time to shutdown.
	for a := shortAttempt.Start(); a.Next(); {
		_, running = caasunitprovisioner.AppWorker(w, "gitlab")
		if !running {
			break
		}
	}
	c.Assert(running, jc.IsFalse)
	workertest.CheckKilled(c, s.unitGetter.watcher)
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

	s.podSpecGetter.watcher.KillErr(errors.New("splat"))
	workertest.CheckKilled(c, s.podSpecGetter.watcher)
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

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.containerBroker.Calls()) > 0 {
			break
		}
	}
	s.containerBroker.CheckCallNames(c, "WatchUnits")

	s.assertUnitChange(c, status.Allocating, status.Allocating)
	s.assertUnitChange(c, status.Allocating, status.Unknown)
}

func (s *WorkerSuite) assertUnitChange(c *gc.C, reported, expected status.Status) {
	s.containerBroker.ResetCalls()
	s.unitUpdater.ResetCalls()
	s.containerBroker.reportedUnitStatus = reported

	select {
	case s.caasUnitsChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.containerBroker.Calls()) > 0 {
			break
		}
	}
	s.containerBroker.CheckCallNames(c, "Units")
	c.Assert(s.containerBroker.Calls()[0].Args, jc.DeepEquals, []interface{}{"gitlab"})

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
				{ProviderId: "u1", Address: "10.0.0.1", Ports: []string(nil), Status: expected.String()},
			},
		},
	})
}
