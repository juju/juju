// Copyright 2015 Canonical Ltd.
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

	"github.com/juju/juju/core/status"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

//go:generate go run go.uber.org/mock/mockgen -package upgradesteps -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -package upgradesteps -destination api_mock_test.go github.com/juju/juju/api/base APICaller
//go:generate go run go.uber.org/mock/mockgen -package upgradesteps -destination lock_mock_test.go github.com/juju/juju/worker/gate Lock
//go:generate go run go.uber.org/mock/mockgen -package upgradesteps -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config,ConfigSetter
//go:generate go run go.uber.org/mock/mockgen -package upgradesteps -destination status_mock_test.go github.com/juju/juju/worker/upgradesteps StatusSetter,UpgradeService

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	testing.IsolationSuite

	agent        *MockAgent
	configSetter *MockConfigSetter
	lock         *MockLock
	clock        *MockClock
	statusSetter *MockStatusSetter
	apiCaller    *MockAPICaller
}

func (s *baseSuite) newBaseWorker(c *gc.C, from, to version.Number) *baseWorker {
	return &baseWorker{
		upgradeCompleteLock: s.lock,
		agent:               s.agent,
		clock:               s.clock,
		apiCaller:           s.apiCaller,
		statusSetter:        s.statusSetter,
		fromVersion:         from,
		toVersion:           to,
		tag:                 names.NewMachineTag("0"),
		performUpgradeSteps: func(from version.Number, targets []upgrades.Target, context upgrades.Context) error {
			return nil
		},
		logger: jujutesting.NewCheckLogger(c),
	}
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agent = NewMockAgent(ctrl)
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

func (s *baseSuite) expectStatus(status status.Status) {
	s.statusSetter.EXPECT().SetStatus(status, gomock.Any(), nil).Return(nil)
}

func (s *baseSuite) expectUpgradeVersion(ver version.Number) {
	s.configSetter.EXPECT().SetUpgradedToVersion(ver)
}
