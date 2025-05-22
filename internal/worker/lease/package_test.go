// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"github.com/juju/tc"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package lease -destination database_mock_test.go github.com/juju/juju/core/database TxnRunner
//go:generate go run go.uber.org/mock/mockgen -typed -package lease -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -typed -package lease -destination prometheus_mock_test.go github.com/prometheus/client_golang/prometheus Registerer

type baseSuite struct {
	testhelpers.IsolationSuite

	logger               logger.Logger
	prometheusRegisterer prometheus.Registerer

	clock *MockClock
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.prometheusRegisterer = NewMockRegisterer(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}

type StubLogger struct{}

func (StubLogger) Errorf(string, ...interface{}) {}
