// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	workermocks "github.com/juju/juju/internal/worker/mocks"
	"github.com/juju/juju/internal/worker/upgradeseries"
	. "github.com/juju/juju/internal/worker/upgradeseries/mocks"
	"github.com/juju/juju/testing"
)

type fakeWatcher struct {
	worker.Worker
	ch <-chan struct{}
}

func (w *fakeWatcher) Changes() watcher.NotifyChannel {
	return w.ch
}

type workerSuite struct {
	testing.BaseSuite

	logger        upgradeseries.Logger
	facade        *MockFacade
	unitDiscovery *MockUnitDiscovery
	upgrader      *MockUpgrader
	notifyWorker  *workermocks.MockWorker

	// The done channel is used by tests to indicate that
	// the worker has accomplished the scenario and can be stopped.
	done chan struct{}
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.logger = loggo.GetLogger("test.upgradeseries")
	s.done = make(chan struct{})
}

// TestFullWorkflow uses the the expectation scenarios from each of the tests
// below to compose a test of the whole upgrade-series scenario, from start
// to finish.
func (s *workerSuite) TestFullWorkflow(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.notify(7)
	s.expectUnitDiscovery()
	s.expectMachineValidateUnitsNotPrepareCompleteNoAction()
	s.expectMachinePrepareStartedUnitsNotPrepareCompleteNoAction()
	s.expectMachinePrepareStartedUnitFilesWrittenProgressPrepareComplete()
	s.expectMachineCompleteStartedUnitsPrepareCompleteUnitsStarted()
	s.expectMachineCompleteStartedUnitsCompleteProgressComplete()
	s.expectMachineCompletedFinishUpgradeSeries()
	s.expectLockNotFoundNoAction()

	w := s.newWorker(c)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesNotStarted,
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) TestLockNotFoundNoAction(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.notify(1)
	s.expectLockNotFoundNoAction()
	w := s.newWorker(c)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesNotStarted,
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) expectLockNotFoundNoAction() {
	// If the lock is not found, no further processing occurs.
	// This is the only call we expect to see.
	s.facade.EXPECT().MachineStatus().Return(model.UpgradeSeriesStatus(""), errors.NewNotFound(nil, "nope"))
}

func (s *workerSuite) TestCompleteNoAction(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// If the workflow is completed, no further processing occurs.
	s.expectUnitDiscovery()
	s.facade.EXPECT().MachineStatus().Return(model.UpgradeSeriesPrepareCompleted, nil)
	s.notify(1)
	w := s.newWorker(c)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesPrepareCompleted,
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) TestMachinePrepareStartedUnitsNotPrepareCompleteNoAction(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.notify(1)
	s.expectUnitDiscovery()
	s.expectMachinePrepareStartedUnitsNotPrepareCompleteNoAction()

	w := s.newWorker(c)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesPrepareStarted,
		"prepared units": []string{"wordpress/0"},
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) expectUnitDiscovery() {
	s.unitDiscovery.EXPECT().Units().Return([]names.UnitTag{
		names.NewUnitTag("wordpress/0"),
		names.NewUnitTag("mysql/0"),
	}, nil)
}

func (s *workerSuite) expectMachineValidateUnitsNotPrepareCompleteNoAction() {
	s.facade.EXPECT().MachineStatus().Return(model.UpgradeSeriesValidate, nil)

	s.expectSetInstanceStatus(model.UpgradeSeriesValidate, "validating units")
}

func (s *workerSuite) expectMachinePrepareStartedUnitsNotPrepareCompleteNoAction() {
	s.facade.EXPECT().MachineStatus().Return(model.UpgradeSeriesPrepareStarted, nil)
	s.expectPinLeadership()

	s.expectSetInstanceStatus(model.UpgradeSeriesPrepareStarted, "preparing units")

	// Only one of the two units has completed preparation.
	s.expectUnitsPrepared("wordpress/0")
}

func (s *workerSuite) TestMachinePrepareStartedUnitFilesWrittenProgressPrepareComplete(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.notify(1)
	s.expectUnitDiscovery()
	s.expectPinLeadership()
	s.expectMachinePrepareStartedUnitFilesWrittenProgressPrepareComplete()
	w := s.newWorker(c)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesPrepareStarted,
		"prepared units": []string{"wordpress/0", "mysql/0"},
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) expectMachinePrepareStartedUnitFilesWrittenProgressPrepareComplete() {
	exp := s.facade.EXPECT()

	exp.MachineStatus().Return(model.UpgradeSeriesPrepareStarted, nil)
	s.expectSetInstanceStatus(model.UpgradeSeriesPrepareStarted, "preparing units")
	s.expectUnitsPrepared("wordpress/0", "mysql/0")

	s.upgrader.EXPECT().PerformUpgrade().Return(nil)
	s.expectSetInstanceStatus(model.UpgradeSeriesPrepareStarted, "completing preparation")

	exp.SetMachineStatus(model.UpgradeSeriesPrepareCompleted, gomock.Any()).Return(nil)
	s.expectSetInstanceStatus(model.UpgradeSeriesPrepareCompleted, "waiting for completion command")
}

func (s *workerSuite) TestMachineCompleteStartedUnitsPrepareCompleteUnitsStarted(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.notify(1)
	s.expectUnitDiscovery()
	s.expectMachineCompleteStartedUnitsPrepareCompleteUnitsStarted()
	w := s.newWorker(c)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesCompleteStarted,
		"prepared units": []string{"wordpress/0", "mysql/0"},
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) expectMachineCompleteStartedUnitsPrepareCompleteUnitsStarted() {
	s.facade.EXPECT().MachineStatus().Return(model.UpgradeSeriesCompleteStarted, nil)
	s.expectSetInstanceStatus(model.UpgradeSeriesCompleteStarted, "waiting for units")
	s.expectUnitsPrepared("wordpress/0", "mysql/0")
	s.facade.EXPECT().StartUnitCompletion(gomock.Any()).Return(nil)
}

func (s *workerSuite) TestMachineCompleteStartedNoUnitsProgressComplete(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// No units for this test.
	s.unitDiscovery.EXPECT().Units().Return(nil, nil)

	exp := s.facade.EXPECT()
	exp.MachineStatus().Return(model.UpgradeSeriesCompleteStarted, nil)
	s.expectSetInstanceStatus(model.UpgradeSeriesCompleteStarted, "waiting for units")

	// Machine with no units - API calls return none, no services discovered.
	exp.UnitsPrepared().Return(nil, nil)
	exp.UnitsCompleted().Return(nil, nil)

	// Progress directly to completed.
	exp.SetMachineStatus(model.UpgradeSeriesCompleted, gomock.Any()).Return(nil)

	s.notify(1)
	w := s.newWorker(c)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesCompleteStarted,
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) TestMachineCompleteStartedUnitsCompleteProgressComplete(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.notify(1)
	s.expectUnitDiscovery()
	s.expectMachineCompleteStartedUnitsCompleteProgressComplete()
	w := s.newWorker(c)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status":  model.UpgradeSeriesCompleteStarted,
		"completed units": []string{"wordpress/0", "mysql/0"},
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) expectMachineCompleteStartedUnitsCompleteProgressComplete() {
	exp := s.facade.EXPECT()

	exp.MachineStatus().Return(model.UpgradeSeriesCompleteStarted, nil)
	s.expectSetInstanceStatus(model.UpgradeSeriesCompleteStarted, "waiting for units")

	// No units are in the prepare-complete state.
	// They have completed their workflow.
	s.expectUnitsPrepared()
	s.facade.EXPECT().UnitsCompleted().Return([]names.UnitTag{
		names.NewUnitTag("wordpress/0"),
		names.NewUnitTag("mysql/0"),
	}, nil)
	exp.SetMachineStatus(model.UpgradeSeriesCompleted, gomock.Any()).Return(nil)
}

func (s *workerSuite) TestMachineCompletedFinishUpgradeSeries(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.notify(1)
	s.expectUnitDiscovery()
	s.expectMachineCompletedFinishUpgradeSeries()
	w := s.newWorker(c)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesCompleted,
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) expectMachineCompletedFinishUpgradeSeries() {
	b := base.MustParseBaseFromString("ubuntu@16.04")
	s.patchHost(b)

	exp := s.facade.EXPECT()
	exp.MachineStatus().Return(model.UpgradeSeriesCompleted, nil)
	s.expectSetInstanceStatus(model.UpgradeSeriesCompleted, "finalising upgrade")
	exp.FinishUpgradeSeries(b).Return(nil)

	s.expectSetInstanceStatus(model.UpgradeSeriesCompleted, "success")
	exp.UnpinMachineApplications().Return(map[string]error{
		"mysql":     nil,
		"wordpress": nil,
	}, nil)
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.facade = NewMockFacade(ctrl)
	s.unitDiscovery = NewMockUnitDiscovery(ctrl)
	s.upgrader = NewMockUpgrader(ctrl)
	s.notifyWorker = workermocks.NewMockWorker(ctrl)

	return ctrl
}

// newWorker creates worker config based on the suite's mocks.
// Any supplied behaviour functions are executed,
// then a new worker is started and returned.
func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	cfg := upgradeseries.Config{
		Logger:          s.logger,
		Facade:          s.facade,
		UnitDiscovery:   s.unitDiscovery,
		UpgraderFactory: func() (upgradeseries.Upgrader, error) { return s.upgrader, nil },
	}

	w, err := upgradeseries.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

// expectUnitsCompleted represents the scenario where the input unit names
// have completed their upgrade-series preparation.
func (s *workerSuite) expectUnitsPrepared(units ...string) {
	tags := make([]names.UnitTag, len(units))
	for i, u := range units {
		tags[i] = names.NewUnitTag(u)
	}
	s.facade.EXPECT().UnitsPrepared().Return(tags, nil)
}

// For individual tests that use a status of UpgradeSeriesPrepare started,
// this will be called each time, but for the full workflow scenario we
// only expect it once. To accommodate this, calls to this method will
// often be in the Test... method instead of its partner expectation
// method.
func (s *workerSuite) expectPinLeadership() {
	s.facade.EXPECT().PinMachineApplications().Return(map[string]error{
		"mysql":     nil,
		"wordpress": nil,
	}, nil)
}

func (s *workerSuite) expectSetInstanceStatus(sts model.UpgradeSeriesStatus, msg string) {
	s.facade.EXPECT().SetInstanceStatus(sts, msg).Return(nil)
}

// cleanKill waits for notifications to be processed, then waits for the input
// worker to be killed cleanly. If either ops time out, the test fails.
func (s *workerSuite) cleanKill(c *gc.C, w worker.Worker) {
	select {
	case <-s.done:
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
	workertest.CleanKill(c, w)
}

func (s *workerSuite) patchHost(b base.Base) {
	upgradeseries.PatchHostBase(s, b)
}

// notify returns a suite behaviour that will cause the upgrade-series watcher
// to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *workerSuite) notify(times int) {
	ch := make(chan struct{})

	go func() {
		for i := 0; i < times; i++ {
			ch <- struct{}{}
		}
		close(s.done)
	}()

	s.notifyWorker.EXPECT().Kill().AnyTimes()
	s.notifyWorker.EXPECT().Wait().Return(nil).AnyTimes()

	s.facade.EXPECT().WatchUpgradeSeriesNotifications().Return(
		&fakeWatcher{
			Worker: s.notifyWorker,
			ch:     ch,
		}, nil)
}
