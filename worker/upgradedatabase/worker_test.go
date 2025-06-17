// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase_test

import (
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/retry"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/upgradedatabase"
	. "github.com/juju/juju/worker/upgradedatabase/mocks"
)

var (
	statusUpgrading = "upgrading database for " + jujuversion.Current.String()
	statusWaiting   = "waiting on primary database upgrade for " + jujuversion.Current.String()
	statusCompleted = fmt.Sprintf("database upgrade for %v completed", jujuversion.Current)
	statusConfirmed = fmt.Sprintf("confirmed primary database upgrade for %s", jujuversion.Current.String())

	logRunning = "running database upgrade for %v on mongodb primary"
	logWaiting = "waiting for database upgrade on mongodb primary"
)

// baseSuite is embedded in both the worker and manifold tests.
// Tests should not go on this suite directly.
type baseSuite struct {
	testing.IsolationSuite

	logger *MockLogger
}

// ignoreLogging turns the suite's mock logger into a sink, with no validation.
// Logs are still emitted via the test logger.
func (s *baseSuite) ignoreLogging(c *gc.C) {
	debugIt := func(message string, args ...interface{}) { logIt(c, loggo.DEBUG, message, args) }
	infoIt := func(message string, args ...interface{}) { logIt(c, loggo.INFO, message, args) }
	errorIt := func(message string, args ...interface{}) { logIt(c, loggo.ERROR, message, args) }

	e := s.logger.EXPECT()
	e.Debugf(gomock.Any(), gomock.Any()).AnyTimes().Do(debugIt)
	e.Infof(gomock.Any(), gomock.Any()).AnyTimes().Do(infoIt)
	e.Errorf(gomock.Any(), gomock.Any()).AnyTimes().Do(errorIt)
}

func logIt(c *gc.C, level loggo.Level, message string, args interface{}) {
	var nArgs []interface{}
	var ok bool
	if nArgs, ok = args.([]interface{}); ok {
		nArgs = append([]interface{}{level}, nArgs...)
	} else {
		nArgs = append([]interface{}{level}, args)
	}

	c.Logf("%s "+message, nArgs...)
}

type workerSuite struct {
	baseSuite

	lock        *MockLock
	agent       *MockAgent
	agentCfg    *MockConfig
	cfgSetter   *MockConfigSetter
	pool        *MockPool
	clock       *MockClock
	upgradeInfo *MockUpgradeInfo
	watcher     *MockNotifyWatcher
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)
	cfg.Tag = names.NewControllerAgentTag("0")
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.UpgradeComplete = nil
	c.Check(cfg.Validate(), jc.Satisfies, errors.IsNotValid)

	cfg = s.getConfig()
	cfg.Tag = nil
	c.Check(cfg.Validate(), jc.Satisfies, errors.IsNotValid)

	cfg = s.getConfig()
	cfg.Agent = nil
	c.Check(cfg.Validate(), jc.Satisfies, errors.IsNotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.Satisfies, errors.IsNotValid)

	cfg = s.getConfig()
	cfg.OpenState = nil
	c.Check(cfg.Validate(), jc.Satisfies, errors.IsNotValid)

	cfg = s.getConfig()
	cfg.PerformUpgrade = nil
	c.Check(cfg.Validate(), jc.Satisfies, errors.IsNotValid)

	cfg = s.getConfig()
	cfg.RetryStrategy = retry.CallArgs{}
	c.Check(cfg.Validate(), jc.Satisfies, errors.IsNotValid)

	cfg = s.getConfig()
	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.Satisfies, errors.IsNotValid)
}

func (s *workerSuite) TestNewLockSameVersionUnlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	s.agentCfg.EXPECT().UpgradedToVersion().Return(jujuversion.Current)
	c.Assert(upgradedatabase.NewLock(s.agentCfg).IsUnlocked(), jc.IsTrue)
}

func (s *workerSuite) TestNewLockOldVersionLocked(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	s.agentCfg.EXPECT().UpgradedToVersion().Return(version.Number{})
	c.Assert(upgradedatabase.NewLock(s.agentCfg).IsUnlocked(), jc.IsFalse)
}

func (s *workerSuite) TestAlreadyCompleteNoWork(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	s.lock.EXPECT().IsUnlocked().Return(true)

	w, err := upgradedatabase.NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestAlreadyUpgradedNoWork(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	s.lock.EXPECT().IsUnlocked().Return(false)
	s.agent.EXPECT().CurrentConfig().Return(s.agentCfg)
	s.agentCfg.EXPECT().UpgradedToVersion().Return(jujuversion.Current)
	s.lock.EXPECT().Unlock()

	w, err := upgradedatabase.NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestNotPrimaryWatchForCompletionSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	s.expectUpgradeRequired(false)

	s.pool.EXPECT().SetStatus("0", status.Started, statusWaiting)

	// Expect a watcher that will fire a change for the initial event
	// and then a change for the watch loop.
	s.upgradeInfo.EXPECT().Watch().Return(s.watcher)
	changes := make(chan struct{}, 2)
	changes <- struct{}{}
	changes <- struct{}{}
	s.watcher.EXPECT().Changes().Return(changes).MinTimes(1)

	// Initial state is UpgradePending
	s.upgradeInfo.EXPECT().Refresh().Return(nil).MinTimes(1)
	s.upgradeInfo.EXPECT().Status().Return(state.UpgradePending)
	// After the first change is retrieved from the channel above, we then say the upgrade is complete
	s.upgradeInfo.EXPECT().Status().Return(state.UpgradeDBComplete)

	s.pool.EXPECT().SetStatus("0", status.Started, statusConfirmed)

	// We don't want to kill the worker while we are in the status observation
	// loop, so we gate on this final expectation.
	finished := make(chan struct{})
	s.lock.EXPECT().Unlock().Do(func() {
		close(finished)
	})

	w, err := upgradedatabase.NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-finished:
	case <-time.After(testing.LongWait):
	}
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestNotPrimaryWatchForCompletionSuccessRunning(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	s.expectUpgradeRequired(false)

	s.pool.EXPECT().SetStatus("0", status.Started, statusWaiting)

	// Expect a watcher that will fire a change for the initial event
	// and then a change for the watch loop.
	s.upgradeInfo.EXPECT().Watch().Return(s.watcher)
	changes := make(chan struct{}, 2)
	changes <- struct{}{}
	changes <- struct{}{}
	s.watcher.EXPECT().Changes().Return(changes).MinTimes(1)

	// Initial state is UpgradePending
	s.upgradeInfo.EXPECT().Refresh().Return(nil).MinTimes(1)
	s.upgradeInfo.EXPECT().Status().Return(state.UpgradePending)
	// After the first change is retrieved from the channel above,
	// we then say the upgrade has moved on to running (non-db) steps.
	s.upgradeInfo.EXPECT().Status().Return(state.UpgradeRunning)

	s.pool.EXPECT().SetStatus("0", status.Started, statusConfirmed)

	// We don't want to kill the worker while we are in the status observation
	// loop, so we gate on this final expectation.
	finished := make(chan struct{})
	s.lock.EXPECT().Unlock().Do(func() {
		close(finished)
	})

	w, err := upgradedatabase.NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-finished:
	case <-time.After(testing.LongWait):
	}
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestNotPrimaryWatchForCompletionTimeout(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectUpgradeRequired(false)

	s.logger.EXPECT().Infof(logWaiting)
	s.pool.EXPECT().SetStatus("0", status.Started, statusWaiting)

	// Expect a watcher that will fire a change for the initial event
	// and then a change for the watch loop.
	s.upgradeInfo.EXPECT().Watch().Return(s.watcher)

	// Have changes available for the first couple of loops, but later
	// stop to allow timeout select case to fire for sure
	changes := make(chan struct{}, 2)
	changes <- struct{}{}
	changes <- struct{}{}
	s.watcher.EXPECT().Changes().Return(changes).AnyTimes()

	timeout := make(chan time.Time, 1)
	s.clock.EXPECT().After(10 * time.Minute).Return(timeout)

	neverTimeout := make(chan time.Time)
	s.clock.EXPECT().After(5 * time.Second).Return(neverTimeout).MaxTimes(1)

	// Primary does not complete the upgrade.
	// After we have gone to the upgrade pending state, trip the timeout.
	s.upgradeInfo.EXPECT().Refresh().Return(nil).AnyTimes()

	// Don't timeout on first few check of status.
	s.upgradeInfo.EXPECT().Status().Return(state.UpgradePending).Times(2)
	s.upgradeInfo.EXPECT().Status().DoAndReturn(func() state.UpgradeStatus {
		// We only care about queueing one in the buffer.
		// Carry on if we're jammed up - we'll fail elsewhere.
		select {
		case timeout <- time.Now():
		default:
		}
		return state.UpgradePending
	}).AnyTimes()

	// We don't want to kill the worker while we are in the status observation
	// loop, so we gate on this final expectation.
	finished := make(chan struct{})
	s.pool.EXPECT().SetStatus("0", status.Error, statusUpgrading).Do(func(string, status.Status, string) {
		close(finished)
	})

	// Note that UpgradeComplete is not unlocked.

	cfg := s.getConfig()
	cfg.Clock = s.clock

	w, err := upgradedatabase.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-finished:
	case <-time.After(testing.LongWait):
	}
	workertest.DirtyKill(c, w)
}

func (s *workerSuite) TestNotPrimaryButPrimaryFinished(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	s.expectUpgradeRequired(false)

	s.pool.EXPECT().SetStatus("0", status.Started, statusWaiting)

	// Expect the watcher to be created, and then the Status is examined.
	// If the status is already complete, there are no calls to the Changes for the watcher.

	// Expect a watcher that will fire a change for the initial event
	// and then a change for the watch loop.
	s.upgradeInfo.EXPECT().Watch().Return(s.watcher)
	// Primary already completed the upgrade.
	s.upgradeInfo.EXPECT().Refresh().Return(nil).MinTimes(1)
	s.upgradeInfo.EXPECT().Status().Return(state.UpgradeDBComplete)

	s.pool.EXPECT().SetStatus("0", status.Started, statusConfirmed)

	// We don't want to kill the worker while we are in the status observation
	// loop, so we gate on this final expectation.
	finished := make(chan struct{})
	s.lock.EXPECT().Unlock().Do(func() {
		close(finished)
	})

	w, err := upgradedatabase.NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-finished:
	case <-time.After(testing.LongWait):
	}
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestNotPrimaryButBecomePrimary(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	// first IsPrimary is false
	s.expectUpgradeRequired(false)

	// We don't want to kill the worker while we are in the status observation
	// loop, so we gate on this final expectation.
	finished := make(chan struct{})
	s.pool.EXPECT().IsPrimary("0").DoAndReturn(func(_ interface{}) (bool, error) {
		// The second isPrimary returns true and marks completion
		close(finished)
		return true, nil
	})

	s.pool.EXPECT().SetStatus("0", status.Started, statusWaiting)

	s.upgradeInfo.EXPECT().Watch().Return(s.watcher)
	s.upgradeInfo.EXPECT().Refresh().Return(nil).MinTimes(1)
	s.upgradeInfo.EXPECT().Status().Return(state.UpgradePending)

	// First clock.After returns the timeout clock
	// After that, returns a clock to wait to re-check primary
	timeout := make(chan time.Time, 1)
	checkPrimary := make(chan time.Time, 1)
	checkPrimary <- time.Now()
	s.clock.EXPECT().After(10 * time.Minute).Return(timeout)
	s.clock.EXPECT().After(5 * time.Second).Return(checkPrimary)

	changes := make(chan struct{}, 1)
	s.watcher.EXPECT().Changes().Return(changes)

	cfg := s.getConfig()
	cfg.Clock = s.clock

	w, err := upgradedatabase.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-finished:
	case <-time.After(testing.LongWait):
	}
	workertest.DirtyKill(c, w)
}

func (s *workerSuite) TestNotPrimaryButBecomePrimaryByError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	// first IsPrimary is false
	s.expectUpgradeRequired(false)

	// We don't want to kill the worker while we are in the status observation
	// loop, so we gate on this final expectation.
	finished := make(chan struct{})
	s.pool.EXPECT().IsPrimary("0").DoAndReturn(func(_ interface{}) (bool, error) {
		// The second isPrimary returns true and marks completion
		close(finished)
		return false, errors.New("Primary has changed")
	})

	s.pool.EXPECT().SetStatus("0", status.Started, statusWaiting)

	s.upgradeInfo.EXPECT().Watch().Return(s.watcher)
	s.upgradeInfo.EXPECT().Refresh().Return(nil).MinTimes(1)
	s.upgradeInfo.EXPECT().Status().Return(state.UpgradePending)

	// First clock.After returns the timeout clock
	// After that, returns a clock to wait to re-check primary
	timeout := make(chan time.Time, 1)
	checkPrimary := make(chan time.Time, 1)
	checkPrimary <- time.Now()
	s.clock.EXPECT().After(10 * time.Minute).Return(timeout)
	s.clock.EXPECT().After(5 * time.Second).Return(checkPrimary)

	changes := make(chan struct{}, 1)
	s.watcher.EXPECT().Changes().Return(changes)

	cfg := s.getConfig()
	cfg.Clock = s.clock

	w, err := upgradedatabase.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-finished:
	case <-time.After(testing.LongWait):
	}
	workertest.DirtyKill(c, w)
}
func (s *workerSuite) TestNotPrimaryButBecomePrimaryAfter2Checks(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	// first IsPrimary is false
	s.expectUpgradeRequired(false)
	// so is the second
	s.pool.EXPECT().IsPrimary("0").Return(false, nil)

	// We don't want to kill the worker while we are in the status observation
	// loop, so we gate on this final expectation.
	finished := make(chan struct{})
	s.pool.EXPECT().IsPrimary("0").DoAndReturn(func(_ interface{}) (bool, error) {
		// The third isPrimary returns true and marks completion
		close(finished)
		return true, nil
	})

	s.pool.EXPECT().SetStatus("0", status.Started, statusWaiting)

	s.upgradeInfo.EXPECT().Watch().Return(s.watcher)
	s.upgradeInfo.EXPECT().Refresh().Return(nil).MinTimes(1)
	s.upgradeInfo.EXPECT().Status().Return(state.UpgradePending)

	// First clock.After returns the timeout clock
	// After that, returns a clock to wait to re-check primary
	timeout := make(chan time.Time, 1)
	checkPrimary := make(chan time.Time, 2)
	checkPrimary <- time.Now()
	checkPrimary <- time.Now()

	s.clock.EXPECT().After(10 * time.Minute).Return(timeout)
	s.clock.EXPECT().After(5 * time.Second).Return(checkPrimary).Times(2)

	changes := make(chan struct{}, 1)
	s.watcher.EXPECT().Changes().Return(changes)

	cfg := s.getConfig()
	cfg.Clock = s.clock

	w, err := upgradedatabase.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-finished:
	case <-time.After(testing.LongWait):
	}
	workertest.DirtyKill(c, w)
}

func (s *workerSuite) TestUpgradedSuccessFirst(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	s.expectUpgradeRequired(true)
	s.expectExecution()

	s.upgradeInfo.EXPECT().SetStatus(state.UpgradeDBComplete).Return(nil)
	s.pool.EXPECT().SetStatus("0", status.Started, statusUpgrading)
	s.pool.EXPECT().SetStatus("0", status.Started, statusCompleted)

	s.lock.EXPECT().Unlock()

	w, err := upgradedatabase.NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestUpgradedRetryThenSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectUpgradeRequired(true)
	s.expectExecution()

	s.logger.EXPECT().Infof(logRunning, jujuversion.Current)
	s.pool.EXPECT().SetStatus("0", status.Started, statusUpgrading)

	cfg := s.getConfig()
	msg := "database upgrade from %v to %v for %q failed (%s): %v"
	s.logger.EXPECT().Errorf(msg, version.Number{}, jujuversion.Current, cfg.Tag, "will retry", gomock.Any())

	s.pool.EXPECT().SetStatus("0", status.Error, statusUpgrading)

	s.upgradeInfo.EXPECT().SetStatus(state.UpgradeDBComplete).Return(nil)
	s.logger.EXPECT().Infof("database upgrade for %v completed successfully.", jujuversion.Current)
	s.pool.EXPECT().SetStatus("0", status.Started, statusCompleted)

	s.lock.EXPECT().Unlock()

	var failedOnce bool
	cfg.PerformUpgrade = func(ver version.Number, targets []upgrades.Target, ctx func() upgrades.Context) error {
		c.Check(ver, gc.Equals, version.Number{})
		c.Check(targets, gc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})

		if !failedOnce {
			failedOnce = true
			return errors.New("boom")
		}
		return nil
	}

	w, err := upgradedatabase.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestUpgradedRetryAllFailed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectUpgradeRequired(true)
	s.expectExecution()

	s.logger.EXPECT().Infof(logRunning, jujuversion.Current)
	s.pool.EXPECT().SetStatus("0", status.Started, statusUpgrading)

	cfg := s.getConfig()
	msg := "database upgrade from %v to %v for %q failed (%s): %v"
	s.logger.EXPECT().Errorf(msg, version.Number{}, jujuversion.Current, cfg.Tag, "will retry", gomock.Any()).MinTimes(1)
	s.logger.EXPECT().Errorf(msg, version.Number{}, jujuversion.Current, cfg.Tag, "giving up", gomock.Any())

	s.pool.EXPECT().SetStatus("0", status.Error, statusUpgrading).MinTimes(1)

	// Note that UpgradeComplete is not unlocked.

	cfg.PerformUpgrade = func(ver version.Number, targets []upgrades.Target, ctx func() upgrades.Context) error {
		c.Check(ver, gc.Equals, version.Number{})
		c.Check(targets, gc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
		return errors.New("boom")
	}

	w, err := upgradedatabase.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	workertest.DirtyKill(c, w)
}

func (s *workerSuite) getConfig() upgradedatabase.Config {
	return upgradedatabase.Config{
		UpgradeComplete: s.lock,
		Tag:             names.NewMachineTag("0"),
		Agent:           s.agent,
		Logger:          s.logger,
		OpenState:       func() (upgradedatabase.Pool, error) { return s.pool, nil },
		PerformUpgrade:  func(version.Number, []upgrades.Target, func() upgrades.Context) error { return nil },
		RetryStrategy:   retry.CallArgs{Clock: clock.WallClock, Delay: time.Millisecond, Attempts: 3},
		Clock:           clock.WallClock,
	}
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.lock = NewMockLock(ctrl)
	s.agent = NewMockAgent(ctrl)
	s.agentCfg = NewMockConfig(ctrl)
	s.cfgSetter = NewMockConfigSetter(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.clock = NewMockClock(ctrl)
	s.upgradeInfo = NewMockUpgradeInfo(ctrl)

	s.pool = NewMockPool(ctrl)
	s.pool.EXPECT().Close().Return(nil).MaxTimes(1)

	s.watcher = NewMockNotifyWatcher(ctrl)
	s.watcher.EXPECT().Stop().Return(nil).MaxTimes(1)

	return ctrl
}

// expectUpgradeRequired sets expectations for a scenario where a database
// upgrade needs to be run.
// The input bool simulates whether we are running the primary MongoDB.
func (s *workerSuite) expectUpgradeRequired(isPrimary bool) {
	fromVersion := version.Number{}

	s.lock.EXPECT().IsUnlocked().Return(false)
	s.pool.EXPECT().IsPrimary("0").Return(isPrimary, nil)
	s.agent.EXPECT().CurrentConfig().Return(s.agentCfg)
	s.agentCfg.EXPECT().UpgradedToVersion().Return(fromVersion)
	s.pool.EXPECT().EnsureUpgradeInfo("0", fromVersion, jujuversion.Current).Return(s.upgradeInfo, nil)
}

// expectExecution simply executes the mutator passed to ChangeConfig.
// In this case it is worker.runUpgradeSteps.
func (s *workerSuite) expectExecution() {
	s.agent.EXPECT().ChangeConfig(gomock.Any()).DoAndReturn(func(f agent.ConfigMutator) error {
		return f(s.cfgSetter)
	})
}
