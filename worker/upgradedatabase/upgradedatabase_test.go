// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase_test

import (
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/upgradedatabase"
)

// baseSuite is embedded in both the worker and manifold tests.
// Tests should not go on this suite directly.
type baseSuite struct {
	testing.IsolationSuite

	logger *upgradedatabase.MockLogger
}

// ignoreLogging turns the suite's mock logger into a sink, with no validation.
// Logs are still emitted via the test logger.
func (s *baseSuite) ignoreLogging(c *gc.C) {
	debugIt := func(message string, args ...any) { logIt(c, loggo.DEBUG, message, args) }
	infoIt := func(message string, args ...any) { logIt(c, loggo.INFO, message, args) }
	warningIt := func(message string, args ...any) { logIt(c, loggo.WARNING, message, args) }
	errorIt := func(message string, args ...any) { logIt(c, loggo.ERROR, message, args) }

	e := s.logger.EXPECT()
	e.Debugf(gomock.Any(), gomock.Any()).AnyTimes().Do(debugIt)
	e.Infof(gomock.Any(), gomock.Any()).AnyTimes().Do(infoIt)
	e.Warningf(gomock.Any(), gomock.Any()).AnyTimes().Do(warningIt)
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

	lock     *upgradedatabase.MockLock
	agent    *upgradedatabase.MockAgent
	agentCfg *upgradedatabase.MockConfig
}

var _ = gc.Suite(&workerSuite{})

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

	w, err := upgradedatabase.NewUpgradeDatabaseWorker(s.getConfig())
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

	w, err := upgradedatabase.NewUpgradeDatabaseWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) getConfig() upgradedatabase.Config {
	return upgradedatabase.Config{
		UpgradeComplete: s.lock,
		Agent:           s.agent,
		Logger:          s.logger,
	}
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.lock = upgradedatabase.NewMockLock(ctrl)
	s.agent = upgradedatabase.NewMockAgent(ctrl)
	s.agentCfg = upgradedatabase.NewMockConfig(ctrl)
	s.logger = upgradedatabase.NewMockLogger(ctrl)

	return ctrl
}
