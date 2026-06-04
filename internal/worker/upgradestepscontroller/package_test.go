// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradestepscontroller

import (
	time "time"

	gomock "github.com/canonical/gomock/gomock"
	names "github.com/juju/names/v6"
	"github.com/juju/tc"

	agent "github.com/juju/juju/agent"
	version "github.com/juju/juju/core/semversion"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/upgradesteps"
)

//go:generate go run github.com/canonical/gomock/mockgen -package upgradestepscontroller -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run github.com/canonical/gomock/mockgen -package upgradestepscontroller -destination api_mock_test.go github.com/juju/juju/api/base APICaller
//go:generate go run github.com/canonical/gomock/mockgen -package upgradestepscontroller -destination lock_mock_test.go github.com/juju/juju/internal/worker/gate Lock
//go:generate go run github.com/canonical/gomock/mockgen -package upgradestepscontroller -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config,ConfigSetter
//go:generate go run github.com/canonical/gomock/mockgen -package upgradestepscontroller -destination upgradeservice_mock_test.go github.com/juju/juju/internal/worker/upgradestepscontroller UpgradeService
//go:generate go run github.com/canonical/gomock/mockgen -package upgradestepscontroller -destination status_mock_test.go github.com/juju/juju/internal/upgradesteps StatusSetter

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
		PreUpgradeSteps: func(_ agent.Config) error {
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

	c.Cleanup(func() {
		s.agent = nil
		s.config = nil
		s.configSetter = nil
		s.lock = nil
		s.clock = nil
		s.statusSetter = nil
		s.apiCaller = nil
	})

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
	case <-c.Context().Done():
		c.Fatalf("timed out waiting to enqueue change")
	}
}
