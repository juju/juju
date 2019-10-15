// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/workertest"

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

	e := s.logger.EXPECT()
	e.Debugf(gomock.Any(), gomock.Any()).AnyTimes().Do(debugIt)
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

	lock     *MockLock
	agent    *MockAgent
	agentCfg *MockConfig
	pool     *MockPool
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
}

func (s *workerSuite) TestAlreadyCompleteNoWork(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.lock.EXPECT().IsUnlocked().Return(true)

	w, err := upgradedatabase.NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestNotPrimaryNoWork(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.lock.EXPECT().IsUnlocked().Return(false)
	s.pool.EXPECT().IsPrimary("0").Return(false, nil)
	s.lock.EXPECT().Unlock()

	w, err := upgradedatabase.NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestAlreadyUpgradedNoWork(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.lock.EXPECT().IsUnlocked().Return(false)
	s.pool.EXPECT().IsPrimary("0").Return(true, nil)
	s.agent.EXPECT().CurrentConfig().Return(s.agentCfg)
	s.agentCfg.EXPECT().UpgradedToVersion().Return(jujuversion.Current)
	s.lock.EXPECT().Unlock()

	w, err := upgradedatabase.NewWorker(s.getConfig())
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
		PerformUpgrade:  func(version.Number, []upgrades.Target, upgrades.Context) error { return nil },
	}
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.lock = NewMockLock(ctrl)
	s.agent = NewMockAgent(ctrl)
	s.agentCfg = NewMockConfig(ctrl)

	s.logger = NewMockLogger(ctrl)
	s.ignoreLogging(c)

	s.pool = NewMockPool(ctrl)
	s.pool.EXPECT().Close().Return(nil).AnyTimes()

	return ctrl
}
