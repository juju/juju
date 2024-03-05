// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"sync/atomic"
	"testing"
	time "time"

	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package eventmultiplexer -destination change_mock_test.go github.com/juju/juju/core/changestream Term
//go:generate go run go.uber.org/mock/mockgen -package eventmultiplexer -destination stream_mock_test.go github.com/juju/juju/internal/changestream/eventmultiplexer Stream
//go:generate go run go.uber.org/mock/mockgen -package eventmultiplexer -destination logger_mock_test.go github.com/juju/juju/internal/changestream/eventmultiplexer Logger
//go:generate go run go.uber.org/mock/mockgen -package eventmultiplexer -destination metrics_mock_test.go github.com/juju/juju/internal/changestream/eventmultiplexer MetricsCollector
//go:generate go run go.uber.org/mock/mockgen -package eventmultiplexer -destination clock_mock_test.go github.com/juju/clock Clock,Timer

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}

type baseSuite struct {
	domaintesting.ControllerSuite

	clock   *MockClock
	logger  *MockLogger
	stream  *MockStream
	metrics *MockMetricsCollector
	term    *MockTerm
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.stream = NewMockStream(ctrl)
	s.metrics = NewMockMetricsCollector(ctrl)
	s.term = NewMockTerm(ctrl)

	return ctrl
}

func (s *baseSuite) expectAnyLogs(c *gc.C) {
	s.logger.EXPECT().Errorf(gomock.Any()).Do(c.Logf).AnyTimes()
	s.logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).Do(c.Logf).AnyTimes()
	s.logger.EXPECT().Infof(gomock.Any()).Do(c.Logf).AnyTimes()
	s.logger.EXPECT().Debugf(gomock.Any()).Do(c.Logf).AnyTimes()
	s.logger.EXPECT().Tracef(gomock.Any()).Do(c.Logf).AnyTimes()
	s.logger.EXPECT().Tracef(gomock.Any(), gomock.Any()).Do(c.Logf).AnyTimes()
	s.logger.EXPECT().IsTraceEnabled().Return(true).AnyTimes()
}

func (s *baseSuite) expectStreamDying(ch <-chan struct{}) {
	s.stream.EXPECT().Dying().Return(ch).AnyTimes()
}

func (s *baseSuite) expectAfter() {
	s.clock.EXPECT().After(gomock.Any()).AnyTimes()
}

func (s *baseSuite) expectTerm(c *gc.C, evts ...changestream.ChangeEvent) {
	s.expectTermInOrder(c, false, evts...)
}

func (s *baseSuite) expectEmptyTerm(c *gc.C, evts ...changestream.ChangeEvent) {
	s.expectTermInOrder(c, true, evts...)
}

func (s *baseSuite) expectTermInOrder(c *gc.C, empty bool, evts ...changestream.ChangeEvent) {
	// The order is important here. We always expect done to be called once
	// all the changes have been read.
	gomock.InOrder(
		s.term.EXPECT().Changes().Return(evts),
		s.term.EXPECT().Done(empty, gomock.Any()),
	)
}

func (s *baseSuite) dispatchTerm(c *gc.C, terms chan<- changestream.Term) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)

		select {
		case terms <- s.term:
		case <-time.After(jujutesting.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()
	return done
}

type changeEvent struct {
	ctype       changestream.ChangeType
	ns, changed string
}

// Type returns the type of change (create, update, delete).
func (c changeEvent) Type() changestream.ChangeType {
	return c.ctype
}

// Namespace returns the namespace of the change. This is normally the
// table name.
func (c changeEvent) Namespace() string {
	return c.ns
}

// Changed returns the changed value of event. This logically can be
// the primary key of the row that was changed or the field of the change
// that was changed.
func (c changeEvent) Changed() string {
	return c.changed
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
