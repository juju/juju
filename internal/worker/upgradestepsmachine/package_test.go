// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradestepsmachine

import (
	time "time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/upgradesteps"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package upgradestepsmachine -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradestepsmachine -destination api_mock_test.go github.com/juju/juju/api/base APICaller
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradestepsmachine -destination lock_mock_test.go github.com/juju/juju/internal/worker/gate Lock
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradestepsmachine -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config,ConfigSetter
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradestepsmachine -destination status_mock_test.go github.com/juju/juju/internal/worker/upgradestepsmachine StatusSetter

type baseSuite struct {
	testhelpers.IsolationSuite

	agent        *MockAgent
	config       *MockConfig
	configSetter *MockConfigSetter
	lock         *MockLock
	clock        *MockClock
	apiCaller    *MockAPICaller
	statusSetter *MockStatusSetter
}

func (s *baseSuite) newBaseWorker(c *tc.C, from, to semversion.Number) *upgradesteps.BaseWorker {
	return &upgradesteps.BaseWorker{
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
		PerformUpgradeSteps: func(from semversion.Number, targets []upgrades.Target, context upgrades.Context) error {
			return nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
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

func (s *baseSuite) expectUpgradeVersion(ver semversion.Number) {
	s.configSetter.EXPECT().SetUpgradedToVersion(ver)
}

func (s *baseSuite) expectStatus(status status.Status) {
	s.statusSetter.EXPECT().SetStatus(gomock.Any(), status, gomock.Any(), nil).Return(nil)
}
