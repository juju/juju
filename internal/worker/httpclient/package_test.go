// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpclient

import (
	"testing"
	"time"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package httpclient -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -typed -package httpclient -destination http_mock_test.go github.com/juju/juju/core/http HTTPClient
//go:generate go run go.uber.org/mock/mockgen -typed -package httpclient -destination httpclient_mock_test.go github.com/juju/juju/internal/worker/httpclient HTTPClientWorker

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	logger logger.Logger

	clock *MockClock
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).AnyTimes()
}
