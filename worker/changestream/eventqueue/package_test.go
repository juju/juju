// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventqueue

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/juju/juju/core/changestream"
	dbtesting "github.com/juju/juju/database/testing"
	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package eventqueue -destination change_mock_test.go github.com/juju/juju/core/changestream ChangeEvent
//go:generate go run github.com/golang/mock/mockgen -package eventqueue -destination stream_mock_test.go github.com/juju/juju/worker/changestream/eventqueue Stream
//go:generate go run github.com/golang/mock/mockgen -package eventqueue -destination logger_mock_test.go github.com/juju/juju/worker/changestream/eventqueue Logger

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	dbtesting.ControllerSuite

	logger      *MockLogger
	stream      *MockStream
	changeEvent *MockChangeEvent
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = NewMockLogger(ctrl)
	s.stream = NewMockStream(ctrl)
	s.changeEvent = NewMockChangeEvent(ctrl)

	return ctrl
}

func (s *baseSuite) expectAnyLogs() {
	s.logger.EXPECT().Tracef(gomock.Any()).AnyTimes()
	s.logger.EXPECT().IsTraceEnabled().Return(false).AnyTimes()
}

func (s *baseSuite) expectChangeEvent(mask changestream.ChangeType, topic string) {
	s.changeEvent.EXPECT().Type().Return(mask).MinTimes(1)
	s.changeEvent.EXPECT().Namespace().Return(topic).MinTimes(1)
}
