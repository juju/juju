// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	stdtesting "testing"
	time "time"

	names "github.com/juju/names/v4"
	"github.com/juju/testing"
	version "github.com/juju/version/v2"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	status "github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/upgrades"
	jujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package upgradesteps -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -package upgradesteps -destination api_mock_test.go github.com/juju/juju/api/base APICaller
//go:generate go run go.uber.org/mock/mockgen -package upgradesteps -destination lock_mock_test.go github.com/juju/juju/worker/gate Lock
//go:generate go run go.uber.org/mock/mockgen -package upgradesteps -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config,ConfigSetter
//go:generate go run go.uber.org/mock/mockgen -package upgradesteps -destination status_mock_test.go github.com/juju/juju/internal/upgradesteps StatusSetter

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	testing.IsolationSuite

	agent        *MockAgent
	config       *MockConfig
	configSetter *MockConfigSetter
	lock         *MockLock
	clock        *MockClock
	apiCaller    *MockAPICaller
	statusSetter *MockStatusSetter
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agent = NewMockAgent(ctrl)
	s.config = NewMockConfig(ctrl)
	s.configSetter = NewMockConfigSetter(ctrl)
	s.lock = NewMockLock(ctrl)
	s.clock = NewMockClock(ctrl)
	s.apiCaller = NewMockAPICaller(ctrl)
	s.statusSetter = NewMockStatusSetter(ctrl)

	return ctrl
}

func (s *baseSuite) expectAnyClock(ch chan time.Time) {
	s.clock.EXPECT().Now().AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(time.Duration) <-chan time.Time {
		return ch
	}).AnyTimes()
}

func (s *baseSuite) expectUpgradeVersion(ver version.Number) {
	s.configSetter.EXPECT().SetUpgradedToVersion(ver)
}

func (s *baseSuite) expectStatus(status status.Status) {
	s.statusSetter.EXPECT().SetStatus(status, gomock.Any(), nil).Return(nil)
}

func (s *baseSuite) newBaseWorker(c *gc.C, from, to version.Number) *BaseWorker {
	return &BaseWorker{
		UpgradeCompleteLock: s.lock,
		Agent:               s.agent,
		Clock:               s.clock,
		APICaller:           s.apiCaller,
		StatusSetter:        s.statusSetter,
		FromVersion:         from,
		ToVersion:           to,
		Tag:                 names.NewMachineTag("0"),
		PreUpgradeSteps: func(_ agent.Config, isController bool) error {
			return nil
		},
		PerformUpgradeSteps: func(from version.Number, targets []upgrades.Target, context upgrades.Context) error {
			return nil
		},
		Logger: jujutesting.NewCheckLogger(c),
	}
}
