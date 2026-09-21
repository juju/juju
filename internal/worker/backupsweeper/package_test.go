// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupsweeper

import (
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run github.com/canonical/gomock/mockgen -package backupsweeper -destination service_mock_test.go github.com/juju/juju/internal/worker/backupsweeper ModelConfigService
//go:generate go run github.com/canonical/gomock/mockgen -package backupsweeper -destination clock_mock_test.go github.com/juju/clock Clock,Timer

type baseSuite struct {
	testhelpers.IsolationSuite

	modelConfig *MockModelConfigService
	clock       *MockClock
	timer       *MockTimer
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelConfig = NewMockModelConfigService(ctrl)
	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
}

// expectTimerRepeated wires a mock timer whose first `times` ticks fire
// immediately, with a final tick that closes done when it is awaited.
func (s *baseSuite) expectTimerRepeated(times int, done chan struct{}) {
	s.clock.EXPECT().NewTimer(gomock.Any()).Return(s.timer)

	s.timer.EXPECT().Chan().DoAndReturn(func() <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}).Times(times)
	s.timer.EXPECT().Reset(gomock.Any()).Times(times)

	// This call will block until the test is done.
	s.timer.EXPECT().Chan().DoAndReturn(func() <-chan time.Time {
		defer func() {
			if done != nil {
				close(done)
			}
		}()

		ch := make(chan time.Time, 1)
		return ch
	})

	s.timer.EXPECT().Stop().Return(true).AnyTimes()
}
