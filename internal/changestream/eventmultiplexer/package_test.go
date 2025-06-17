// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"sync/atomic"
	time "time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/changestream"
	domaintesting "github.com/juju/juju/domain/schema/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package eventmultiplexer -destination change_mock_test.go github.com/juju/juju/core/changestream Term
//go:generate go run go.uber.org/mock/mockgen -typed -package eventmultiplexer -destination stream_mock_test.go github.com/juju/juju/internal/changestream/eventmultiplexer Stream
//go:generate go run go.uber.org/mock/mockgen -typed -package eventmultiplexer -destination metrics_mock_test.go github.com/juju/juju/internal/changestream/eventmultiplexer MetricsCollector
//go:generate go run go.uber.org/mock/mockgen -typed -package eventmultiplexer -destination clock_mock_test.go github.com/juju/clock Clock,Timer

type baseSuite struct {
	domaintesting.ControllerSuite

	stream  *MockStream
	metrics *MockMetricsCollector
	term    *MockTerm

	clock *MockClock
	timer *MockTimer

	timerCh chan time.Time
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.stream = NewMockStream(ctrl)
	s.metrics = NewMockMetricsCollector(ctrl)
	s.term = NewMockTerm(ctrl)

	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)

	s.clock.EXPECT().Now().AnyTimes()
	s.clock.EXPECT().NewTimer(gomock.Any()).Return(s.timer).AnyTimes()
	s.timer.EXPECT().Stop().AnyTimes()
	s.timer.EXPECT().Reset(gomock.Any()).AnyTimes()

	s.timerCh = make(chan time.Time, 1)

	s.timer.EXPECT().Chan().Return(s.timerCh).AnyTimes()

	c.Cleanup(func() {
		s.stream = nil
		s.metrics = nil
		s.term = nil
		s.clock = nil
		s.timer = nil
	})

	return ctrl
}

func (s *baseSuite) expectStreamDying(ch <-chan struct{}) {
	s.stream.EXPECT().Dying().Return(ch).AnyTimes()
}

func (s *baseSuite) expectAfter() {
	s.clock.EXPECT().After(gomock.Any()).AnyTimes()
}

func (s *baseSuite) expectTerm(c *tc.C, evts ...changestream.ChangeEvent) {
	s.expectTermInOrder(c, false, evts...)
}

func (s *baseSuite) expectEmptyTerm(c *tc.C, evts ...changestream.ChangeEvent) {
	s.expectTermInOrder(c, true, evts...)
}

func (s *baseSuite) expectTermInOrder(c *tc.C, done bool, evts ...changestream.ChangeEvent) {
	// The order is important here. We always expect done to be called once
	// all the changes have been read.
	gomock.InOrder(
		s.term.EXPECT().Changes().Return(evts),
		s.term.EXPECT().Done(done, gomock.Any()),
	)
}

func (s *baseSuite) expectAnyTermInOrder(c *tc.C, evts ...changestream.ChangeEvent) {
	s.term.EXPECT().Changes().Return(evts).AnyTimes()
	s.term.EXPECT().Done(gomock.Any(), gomock.Any()).AnyTimes()
}

func (s *baseSuite) dispatchTerm(c *tc.C, terms chan<- changestream.Term) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)

		select {
		case terms <- s.term:
		case <-c.Context().Done():
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
