// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/upgradedatabase"
	. "github.com/juju/juju/worker/upgradedatabase/mocks"
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

	lock      *MockLock
	agent     *MockAgent
	agentCfg  *MockConfig
	cfgSetter *MockConfigSetter
	pool      *MockPool
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
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
	cfg.RetryStrategy = utils.AttemptStrategy{}
	c.Check(cfg.Validate(), jc.Satisfies, errors.IsNotValid)
}

func (s *workerSuite) TestAlreadyCompleteNoWork(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	s.lock.EXPECT().IsUnlocked().Return(true)

	w, err := upgradedatabase.NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestNotPrimaryNoWork(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	s.lock.EXPECT().IsUnlocked().Return(false)
	s.pool.EXPECT().IsPrimary("0").Return(false, nil)
	s.lock.EXPECT().Unlock()

	w, err := upgradedatabase.NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestAlreadyUpgradedNoWork(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.ignoreLogging(c)

	s.lock.EXPECT().IsUnlocked().Return(false)
	s.pool.EXPECT().IsPrimary("0").Return(true, nil)
	s.agent.EXPECT().CurrentConfig().Return(s.agentCfg)
	s.agentCfg.EXPECT().UpgradedToVersion().Return(jujuversion.Current)
	s.lock.EXPECT().Unlock()

	w, err := upgradedatabase.NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestUpgradedSuccessFirst(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.lock.EXPECT().IsUnlocked().Return(false)
	s.pool.EXPECT().IsPrimary("0").Return(true, nil)

	s.agent.EXPECT().CurrentConfig().Return(s.agentCfg)
	s.agentCfg.EXPECT().UpgradedToVersion().Return(version.Number{})
	s.expectExecution()
	s.pool.EXPECT().SetStatus("0", status.Started, "upgrading database to "+jujuversion.Current.String())
	s.logger.EXPECT().Infof("database upgrade to %v completed successfully.", jujuversion.Current)
	s.pool.EXPECT().SetStatus("0", status.Started, "")

	s.lock.EXPECT().Unlock()

	w, err := upgradedatabase.NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestUpgradedRetryThenSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.lock.EXPECT().IsUnlocked().Return(false)
	s.pool.EXPECT().IsPrimary("0").Return(true, nil)

	s.agent.EXPECT().CurrentConfig().Return(s.agentCfg)
	s.agentCfg.EXPECT().UpgradedToVersion().Return(version.Number{})
	s.expectExecution()
	s.pool.EXPECT().SetStatus("0", status.Started, "upgrading database to "+jujuversion.Current.String())

	cfg := s.getConfig()
	msg := "database upgrade from %v to %v for %q failed (%s): %v"
	s.logger.EXPECT().Errorf(msg, version.Number{}, jujuversion.Current, cfg.Tag, "will retry", gomock.Any())

	s.pool.EXPECT().SetStatus("0", status.Error, "upgrading database to "+jujuversion.Current.String())
	s.logger.EXPECT().Infof("database upgrade to %v completed successfully.", jujuversion.Current)
	s.pool.EXPECT().SetStatus("0", status.Started, "")

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

	s.lock.EXPECT().IsUnlocked().Return(false)
	s.pool.EXPECT().IsPrimary("0").Return(true, nil)

	s.agent.EXPECT().CurrentConfig().Return(s.agentCfg)
	s.agentCfg.EXPECT().UpgradedToVersion().Return(version.Number{})
	s.expectExecution()
	s.pool.EXPECT().SetStatus("0", status.Started, "upgrading database to "+jujuversion.Current.String())

	cfg := s.getConfig()
	msg := "database upgrade from %v to %v for %q failed (%s): %v"
	s.logger.EXPECT().Errorf(msg, version.Number{}, jujuversion.Current, cfg.Tag, "will retry", gomock.Any()).MinTimes(1)
	s.logger.EXPECT().Errorf(msg, version.Number{}, jujuversion.Current, cfg.Tag, "giving up", gomock.Any())

	s.pool.EXPECT().SetStatus("0", status.Error, "upgrading database to "+jujuversion.Current.String()).MinTimes(1)

	// Note that UpgradeComplete is not unlocked.

	cfg.PerformUpgrade = func(ver version.Number, targets []upgrades.Target, ctx func() upgrades.Context) error {
		c.Check(ver, gc.Equals, version.Number{})
		c.Check(targets, gc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
		return errors.New("boom")
	}

	w, err := upgradedatabase.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) getConfig() upgradedatabase.Config {
	return upgradedatabase.Config{
		UpgradeComplete: s.lock,
		Tag:             names.NewMachineTag("0"),
		Agent:           s.agent,
		Logger:          s.logger,
		OpenState:       func() (upgradedatabase.Pool, error) { return s.pool, nil },
		PerformUpgrade:  func(version.Number, []upgrades.Target, func() upgrades.Context) error { return nil },
		RetryStrategy:   utils.AttemptStrategy{Delay: time.Millisecond, Min: 3},
	}
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.lock = NewMockLock(ctrl)
	s.agent = NewMockAgent(ctrl)
	s.agentCfg = NewMockConfig(ctrl)
	s.cfgSetter = NewMockConfigSetter(ctrl)
	s.logger = NewMockLogger(ctrl)

	s.pool = NewMockPool(ctrl)
	s.pool.EXPECT().Close().Return(nil).AnyTimes()

	return ctrl
}

// This simply executes the mutator passed to ChangeConfig.
// In this case it is worker.runUpgradeSteps.
func (s *workerSuite) expectExecution() {
	s.agent.EXPECT().ChangeConfig(gomock.Any()).DoAndReturn(func(f agent.ConfigMutator) error {
		return f(s.cfgSetter)
	})
}
