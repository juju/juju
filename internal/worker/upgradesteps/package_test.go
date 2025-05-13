// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	stdtesting "testing"
	time "time"

	names "github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	agent "github.com/juju/juju/agent"
	version "github.com/juju/juju/core/semversion"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/upgradesteps"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package upgradesteps -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradesteps -destination api_mock_test.go github.com/juju/juju/api/base APICaller
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradesteps -destination lock_mock_test.go github.com/juju/juju/internal/worker/gate Lock
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradesteps -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config,ConfigSetter
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradesteps -destination upgradeservice_mock_test.go github.com/juju/juju/internal/worker/upgradesteps UpgradeService
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradesteps -destination status_mock_test.go github.com/juju/juju/internal/upgradesteps StatusSetter

func TestAll(t *stdtesting.T) {
	tc.TestingT(t)
}

type baseSuite struct {
	testhelpers.IsolationSuite

	agent        *MockAgent
	config       *MockConfig
	configSetter *MockConfigSetter
	lock         *MockLock
	clock        *MockClock
	statusSetter *MockStatusSetter
	apiCaller    *MockAPICaller
}

func (s *baseSuite) newBaseWorker(c *tc.C, from, to version.Number) *upgradesteps.BaseWorker {
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
		PerformUpgradeSteps: func(from version.Number, targets []upgrades.Target, context upgrades.Context) error {
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
	s.statusSetter = NewMockStatusSetter(ctrl)
	s.apiCaller = NewMockAPICaller(ctrl)

	return ctrl
}

func (s *baseSuite) expectAnyClock(ch chan time.Time) {
	s.clock.EXPECT().Now().AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(time.Duration) <-chan time.Time {
		return ch
	}).AnyTimes()
}

func (s *baseSuite) dispatchChange(c *tc.C, ch chan struct{}) {
	// Send initial event.
	select {
	case ch <- struct{}{}:
	case <-time.After(testhelpers.ShortWait):
		c.Fatalf("timed out waiting to enqueue change")
	}
}
