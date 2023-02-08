// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"testing"

	"github.com/golang/mock/gomock"
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package changestream -destination stream_mock_test.go github.com/juju/juju/worker/changestream ChangeStream,DBGetter,DBStream
//go:generate go run github.com/golang/mock/mockgen -package changestream -destination logger_mock_test.go github.com/juju/juju/worker/changestream Logger
//go:generate go run github.com/golang/mock/mockgen -package changestream -destination clock_mock_test.go github.com/juju/clock Clock

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	dbGetter *MockDBGetter
	clock    *MockClock
	logger   *MockLogger
	dbStream *MockDBStream
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.dbGetter = NewMockDBGetter(ctrl)
	s.clock = NewMockClock(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.dbStream = NewMockDBStream(ctrl)

	return ctrl
}

func (s *baseSuite) expectAnyLogs() {
	s.logger.EXPECT().Errorf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Warningf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Debugf(gomock.Any()).AnyTimes()
}
