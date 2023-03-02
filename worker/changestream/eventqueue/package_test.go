// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventqueue

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/juju/core/changestream"
	dbtesting "github.com/juju/juju/database/testing"
	jujutesting "github.com/juju/juju/testing"
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
	s.logger.EXPECT().Infof(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Tracef(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Tracef(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().IsTraceEnabled().Return(true).AnyTimes()
}

func (s *baseSuite) expectChangeEvent(mask changestream.ChangeType, topic string) {
	s.changeEvent.EXPECT().Type().Return(mask).MinTimes(1)
	s.changeEvent.EXPECT().Namespace().Return(topic).MinTimes(1)
}

func (s *baseSuite) dispatchEvent(c *gc.C, changes chan<- changestream.ChangeEvent) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)

		select {
		case changes <- s.changeEvent:
		case <-time.After(jujutesting.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()
	return done
}

type waitGroup struct {
	ch            chan struct{}
	state, amount uint64
}

func newWaitGroup(amount uint64) *waitGroup {
	return &waitGroup{
		ch:     make(chan struct{}),
		amount: amount,
	}
}

func (w *waitGroup) Done() {
	if atomic.AddUint64(&w.state, 1) == w.amount {
		close(w.ch)
	}
}

func (w *waitGroup) Wait() <-chan struct{} {
	return w.ch
}
