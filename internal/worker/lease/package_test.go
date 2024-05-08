// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"testing"

	jujutesting "github.com/juju/testing"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	jujujujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package lease -destination database_mock_test.go github.com/juju/juju/core/database TxnRunner
//go:generate go run go.uber.org/mock/mockgen -typed -package lease -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -typed -package lease -destination prometheus_mock_test.go github.com/prometheus/client_golang/prometheus Registerer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	logger               Logger
	prometheusRegisterer prometheus.Registerer

	clock *MockClock
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.prometheusRegisterer = NewMockRegisterer(ctrl)

	s.logger = jujujujutesting.CheckLogger{
		Log: c,
	}

	return ctrl
}

type StubLogger struct{}

func (StubLogger) Errorf(string, ...interface{}) {}
