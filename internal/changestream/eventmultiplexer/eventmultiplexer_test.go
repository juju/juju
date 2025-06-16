// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"sync"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/changestream"
	changestreamtesting "github.com/juju/juju/core/changestream/testing"
	"github.com/juju/juju/core/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

const (
	// We need to ensure that we never witness a change term. We've picked
	// an arbitrary timeout of 1 second, which should be more than enough
	// time for the worker to process the change.
	witnessChangeLongDuration  = time.Second
	witnessChangeShortDuration = witnessChangeLongDuration / 2
)

type eventMultiplexerSuite struct {
	baseSuite
}

func TestEventMultiplexerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &eventMultiplexerSuite{})
}

func (s *eventMultiplexerSuite) TestSubscribe(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).AnyTimes()

	s.metrics.EXPECT().SubscriptionsInc()
	s.metrics.EXPECT().SubscriptionsDec()

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestreamtesting.Create))
	c.Assert(err, tc.ErrorIsNil)

	s.unsubscribe(c, sub)
}

func (s *eventMultiplexerSuite) TestDispatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	s.metrics.EXPECT().SubscriptionsInc()
	s.metrics.EXPECT().SubscriptionsDec()
	s.clock.EXPECT().Now().MinTimes(1)
	s.metrics.EXPECT().DispatchDurationObserve(gomock.Any(), false)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestreamtesting.Create))
	c.Assert(err, tc.ErrorIsNil)

	s.expectTerm(c, changeEvent{
		ctype:   changestreamtesting.Create,
		ns:      "topic",
		changed: "1",
	})
	s.dispatchTerm(c, terms)

	var changes []changestream.ChangeEvent
	select {
	case changes = <-sub.Changes():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	c.Assert(changes, tc.HasLen, 1)
	c.Check(changes[0].Type(), tc.DeepEquals, changestreamtesting.Create)
	c.Check(changes[0].Namespace(), tc.DeepEquals, "topic")
	c.Check(changes[0].Changed(), tc.Equals, "1")

	s.unsubscribe(c, sub)
}

func (s *eventMultiplexerSuite) TestMultipleDispatch(c *tc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestreamtesting.Update))
}

func (s *eventMultiplexerSuite) TestMultipleDispatchWithNoOptions(c *tc.C) {
	s.testMultipleDispatch(c)
}

func (s *eventMultiplexerSuite) TestMultipleDispatchWithMultipleMasks(c *tc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestreamtesting.Create|changestreamtesting.Update))
}

func (s *eventMultiplexerSuite) TestMultipleDispatchWithMultipleOptions(c *tc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestreamtesting.Update), changestream.Namespace("topic", changestreamtesting.Create))
}

func (s *eventMultiplexerSuite) TestMultipleDispatchWithOverlappingOptions(c *tc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestreamtesting.Update), changestream.Namespace("topic", changestreamtesting.Update|changestreamtesting.Create))
}

func (s *eventMultiplexerSuite) TestMultipleDispatchWithDuplicateOptions(c *tc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestreamtesting.Update), changestream.Namespace("topic", changestreamtesting.Update))
}

func (s *eventMultiplexerSuite) TestSubscribeWithMatchingFilter(c *tc.C) {
	s.testMultipleDispatch(c, changestream.FilteredNamespace("topic", changestreamtesting.Update, func(event changestream.ChangeEvent) bool {
		return event.Namespace() == "topic"
	}))
}

func (s *eventMultiplexerSuite) testMultipleDispatch(c *tc.C, opts ...changestream.SubscriptionOption) {
	defer s.setupMocks(c).Finish()

	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc().Times(10)
	s.metrics.EXPECT().SubscriptionsDec().Times(10)
	s.clock.EXPECT().Now().MinTimes(1)
	s.metrics.EXPECT().DispatchDurationObserve(gomock.Any(), false)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	s.expectTerm(c, changeEvent{
		ctype:   changestreamtesting.Update,
		ns:      "topic",
		changed: "1",
	})

	subs := make([]changestream.Subscription, 10)
	for i := 0; i < len(subs); i++ {
		sub, err := queue.Subscribe(opts...)
		c.Assert(err, tc.ErrorIsNil)

		subs[i] = sub
	}

	done := s.dispatchTerm(c, terms)
	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for dispatching event")
	}

	// The subscriptions are guaranteed to be out of order, so we need to just
	// wait on them all, and then check that they all got the event.
	wg := newWaitGroup(uint64(len(subs)))
	for i, sub := range subs {
		go func(i int, sub changestream.Subscription) {
			defer wg.Done()

			select {
			case events := <-sub.Changes():
				c.Assert(events, tc.HasLen, 1)
				c.Check(events[0].Type(), tc.DeepEquals, changestreamtesting.Update)
				c.Check(events[0].Namespace(), tc.DeepEquals, "topic")
			case <-c.Context().Done():
			}
		}(i, sub)
	}

	select {
	case <-wg.Wait():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for all events")
	}

	for _, sub := range subs {
		s.unsubscribe(c, sub)
	}

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestUnsubscribeTwice(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc()
	s.metrics.EXPECT().SubscriptionsDec()
	s.clock.EXPECT().Now().MinTimes(1)
	s.metrics.EXPECT().DispatchDurationObserve(gomock.Any(), false)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestreamtesting.Create))
	c.Assert(err, tc.ErrorIsNil)

	s.expectTerm(c, changeEvent{
		ctype:   changestreamtesting.Create,
		ns:      "topic",
		changed: "1",
	})
	s.dispatchTerm(c, terms)

	select {
	case <-sub.Changes():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	s.unsubscribe(c, sub)
	s.unsubscribe(c, sub)

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestTopicDoesNotMatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc()
	s.metrics.EXPECT().SubscriptionsDec()

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestreamtesting.Create))
	c.Assert(err, tc.ErrorIsNil)

	s.expectEmptyTerm(c, changeEvent{
		ctype:   changestreamtesting.Create,
		ns:      "foo",
		changed: "1",
	})
	done := s.dispatchTerm(c, terms)
	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	select {
	case <-sub.Changes():
		c.Fatal("witnessed change when expected none")
	case <-time.After(witnessChangeShortDuration):
	}

	s.unsubscribe(c, sub)

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestTopicMatchesOne(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc().Times(2)
	s.metrics.EXPECT().SubscriptionsDec().Times(2)
	s.clock.EXPECT().Now().MinTimes(1)
	s.metrics.EXPECT().DispatchDurationObserve(gomock.Any(), false)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	sub0, err := queue.Subscribe(changestream.Namespace("foo", changestreamtesting.Create))
	c.Assert(err, tc.ErrorIsNil)

	sub1, err := queue.Subscribe(changestream.Namespace("topic", changestreamtesting.Create))
	c.Assert(err, tc.ErrorIsNil)

	s.expectTerm(c, changeEvent{
		ctype:   changestreamtesting.Create,
		ns:      "topic",
		changed: "1",
	})
	done := s.dispatchTerm(c, terms)
	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	select {
	case <-sub1.Changes():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	select {
	case <-sub0.Changes():
		c.Fatal("witnessed change on sub0 when expected none")
	case <-time.After(witnessChangeShortDuration):
	}

	s.unsubscribe(c, sub0)
	s.unsubscribe(c, sub1)

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestSubscriptionDoneWhenEventQueueKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc()
	s.clock.EXPECT().Now().MinTimes(1)
	// We might encounter a dispatch error, therefore we cannot hard-code
	// a false on the second argument of DispatchDurationObserve.
	s.metrics.EXPECT().DispatchDurationObserve(gomock.Any(), gomock.Any())

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestreamtesting.Create))
	c.Assert(err, tc.ErrorIsNil)

	s.expectTerm(c, changeEvent{
		ctype:   changestreamtesting.Create,
		ns:      "topic",
		changed: "1",
	})
	done := s.dispatchTerm(c, terms)
	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}
	workertest.CleanKill(c, queue)

	select {
	case <-sub.Done():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}
}

func (s *eventMultiplexerSuite) TestUnsubscribeOfOtherSubscription(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc().Times(2)
	s.metrics.EXPECT().SubscriptionsDec().Times(4)
	s.clock.EXPECT().Now().MinTimes(1)
	s.metrics.EXPECT().DispatchDurationObserve(gomock.Any(), false)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	subs := make([]changestream.Subscription, 2)
	for i := range len(subs) {
		sub, err := queue.Subscribe(changestream.Namespace("topic", changestreamtesting.Create))
		c.Assert(err, tc.ErrorIsNil)
		subs[i] = sub
	}

	s.expectTerm(c, changeEvent{
		ctype:   changestreamtesting.Create,
		ns:      "topic",
		changed: "1",
	})
	s.dispatchTerm(c, terms)

	// The subscriptions are guaranteed to be out of order, so we need to just
	// wait on them all, and then check that they all got the event.
	wg := newWaitGroup(uint64(len(subs)))
	for i, sub := range subs {
		go func(i int, sub changestream.Subscription) {
			defer wg.Done()

			select {
			case <-sub.Changes():
				subs[len(subs)-1-i].Unsubscribe()
			case <-c.Context().Done():
			}
		}(i, sub)
	}

	select {
	case <-wg.Wait():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for all events")
	}

	for _, sub := range subs {
		select {
		case <-sub.Done():
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for event")
		}
	}

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestUnsubscribeOfOtherSubscriptionInAnotherGoroutine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc().Times(2)
	s.metrics.EXPECT().SubscriptionsDec().Times(2)
	s.clock.EXPECT().Now().MinTimes(1)
	s.metrics.EXPECT().DispatchDurationObserve(gomock.Any(), false)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	subs := make([]changestream.Subscription, 2)
	for i := 0; i < len(subs); i++ {

		sub, err := queue.Subscribe(changestream.Namespace("topic", changestreamtesting.Create))
		c.Assert(err, tc.ErrorIsNil)
		subs[i] = sub
	}

	s.expectTerm(c, changeEvent{
		ctype:   changestreamtesting.Create,
		ns:      "topic",
		changed: "1",
	})
	s.dispatchTerm(c, terms)

	// The subscriptions are guaranteed to be out of order, so we need to just
	// wait on them all, and then check that they all got the event.
	wg := newWaitGroup(uint64(len(subs)))
	for i, sub := range subs {
		go func(sub changestream.Subscription, i int) {
			select {
			case <-sub.Changes():
				go func() {
					defer wg.Done()

					subs[len(subs)-1-i].Unsubscribe()
				}()
			case <-c.Context().Done():
			}
		}(sub, i)
	}

	select {
	case <-wg.Wait():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for all events")
	}

	for _, sub := range subs {
		select {
		case <-sub.Done():
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for event")
		}
	}

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestStreamDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{})
	s.expectStreamDying(ch)

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc().Times(2)
	s.clock.EXPECT().Now().MinTimes(2)
	s.metrics.EXPECT().DispatchDurationObserve(gomock.Any(), false)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	subs := make([]changestream.Subscription, 2)
	for i := 0; i < len(subs); i++ {
		sub, err := queue.Subscribe(changestream.Namespace("topic", changestreamtesting.Create))
		c.Assert(err, tc.ErrorIsNil)
		subs[i] = sub
	}

	s.expectTerm(c, changeEvent{
		ctype:   changestreamtesting.Create,
		ns:      "topic",
		changed: "1",
	})
	s.dispatchTerm(c, terms)

	// The subscriptions are guaranteed to be out of order, so we need to just
	// wait on them all, and then check that they all got the event.
	wg := newWaitGroup(uint64(len(subs)))
	for i, sub := range subs {
		go func(sub changestream.Subscription, i int) {
			select {
			case <-sub.Changes():
				go func() {
					defer wg.Done()
				}()
			case <-c.Context().Done():
			}
		}(sub, i)
	}

	select {
	case <-wg.Wait():
		close(ch)

	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for all events")
	}

	// Give a grace period for the stream to die and then kill the queue. This
	// should ensure that all the subscriptions are cleaned up.
	<-time.After(testing.ShortWait)
	workertest.CleanKill(c, queue)

	for _, sub := range subs {
		select {
		case <-sub.Done():
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for event")
		}
	}
}

func (s *eventMultiplexerSuite) TestStreamDyingWhilstDispatching(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{})
	s.expectStreamDying(ch)

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc().Times(2)
	s.metrics.EXPECT().SubscriptionsDec().MinTimes(1)
	s.clock.EXPECT().Now().MinTimes(1)
	s.metrics.EXPECT().DispatchDurationObserve(gomock.Any(), false)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	subs := make([]changestream.Subscription, 2)
	for i := 0; i < len(subs); i++ {
		sub, err := queue.Subscribe(changestream.Namespace("topic", changestreamtesting.Create))
		c.Assert(err, tc.ErrorIsNil)
		subs[i] = sub
	}

	s.expectTerm(c, changeEvent{
		ctype:   changestreamtesting.Create,
		ns:      "topic",
		changed: "1",
	})
	s.dispatchTerm(c, terms)

	var once sync.Once

	// The subscriptions are guaranteed to be out of order, so we need to just
	// wait on them all, and then check that they all got the event.
	wg := newWaitGroup(uint64(len(subs)))
	for i, sub := range subs {
		go func(sub changestream.Subscription, i int) {
			select {
			case _, ok := <-sub.Changes():
				if !ok {
					wg.Done()
					return
				}

				go func() {
					defer wg.Done()

					sub.Unsubscribe()

					// This will cause a race to close the channel, but that's
					// fine, as we're only interested in the first one.
					once.Do(func() {
						close(ch)
					})

				}()
			case <-c.Context().Done():
			}
		}(sub, i)
	}

	select {
	case <-wg.Wait():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for all events")
	}

	// Give a grace period for the stream to die and then kill the queue. This
	// should ensure that all the subscriptions are cleaned up.
	<-time.After(testing.ShortWait)
	workertest.CleanKill(c, queue)

	for _, sub := range subs {
		select {
		case <-sub.Done():
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for event")
		}
	}
}

func (s *eventMultiplexerSuite) TestStreamDyingOnStartup(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{})
	s.expectStreamDying(ch)

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	close(ch)

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestStreamDyingOnSubscribe(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{})
	s.expectStreamDying(ch)

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	// We don't care for the metrics recording here, as we might not
	// have recorded the metrics in time before dying.
	s.metrics.EXPECT().SubscriptionsInc().AnyTimes()
	s.metrics.EXPECT().SubscriptionsDec().AnyTimes()

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	close(ch)

	// Give a grace period for the stream to die and then kill the queue. This
	// should ensure that all the subscriptions are cleaned up.
	<-time.After(testing.ShortWait)
	workertest.CleanKill(c, queue)

	sub, err := queue.Subscribe()
	c.Assert(err, tc.ErrorIs, database.ErrEventMultiplexerDying)
	c.Check(sub, tc.IsNil)
}

func (s *eventMultiplexerSuite) TestReportWithAllSubscriptions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAfter()
	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc().Times(10)
	s.metrics.EXPECT().SubscriptionsDec().Times(10)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	var subs []changestream.Subscription
	for i := 0; i < 10; i++ {
		sub, err := queue.Subscribe()
		c.Assert(err, tc.ErrorIsNil)

		subs = append(subs, sub)
	}

	// Sync point. Wait for sometime to let the subscriptions be registered.
	time.Sleep(time.Millisecond * 100)

	c.Check(queue.Report(), tc.DeepEquals, map[string]any{
		"subscriptions":        10,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    10,
		"dispatch-error-count": 0,
	})

	for _, sub := range subs {
		s.unsubscribe(c, sub)
	}

	c.Check(queue.Report(), tc.DeepEquals, map[string]any{
		"subscriptions":        0,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestReportWithTopicSubscriptions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAfter()
	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc().Times(10)
	s.metrics.EXPECT().SubscriptionsDec().Times(10)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	var subs []changestream.Subscription
	for i := 0; i < 10; i++ {
		sub, err := queue.Subscribe(changestream.Namespace("topic", changestreamtesting.Create))
		c.Assert(err, tc.ErrorIsNil)

		subs = append(subs, sub)
	}

	// Sync point. Wait for sometime to let the subscriptions be registered.
	time.Sleep(time.Millisecond * 100)

	c.Check(queue.Report(), tc.DeepEquals, map[string]any{
		"subscriptions":        10,
		"subscriptions-by-ns":  1,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	for _, sub := range subs {
		s.unsubscribe(c, sub)
	}

	c.Check(queue.Report(), tc.DeepEquals, map[string]any{
		"subscriptions":        0,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestReportWithMultipleTopicSubscriptions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAfter()
	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc().Times(10)
	s.metrics.EXPECT().SubscriptionsDec().Times(10)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	var subs []changestream.Subscription
	for i := 0; i < 10; i++ {
		sub, err := queue.Subscribe(
			changestream.Namespace("topic", changestreamtesting.Create),
			changestream.Namespace("foo", changestreamtesting.Update),
		)
		c.Assert(err, tc.ErrorIsNil)

		subs = append(subs, sub)
	}

	// Sync point. Wait for sometime to let the subscriptions be registered.
	time.Sleep(time.Millisecond * 100)

	c.Check(queue.Report(), tc.DeepEquals, map[string]any{
		"subscriptions":        10,
		"subscriptions-by-ns":  2,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	for _, sub := range subs {
		s.unsubscribe(c, sub)
	}

	c.Check(queue.Report(), tc.DeepEquals, map[string]any{
		"subscriptions":        0,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestReportWithDuplicateTopicSubscriptions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAfter()
	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc().Times(10)
	s.metrics.EXPECT().SubscriptionsDec().Times(10)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	var subs []changestream.Subscription
	for i := 0; i < 10; i++ {
		sub, err := queue.Subscribe(
			changestream.Namespace("topic", changestreamtesting.Update),
			changestream.Namespace("topic", changestreamtesting.Update),
		)
		c.Assert(err, tc.ErrorIsNil)

		subs = append(subs, sub)
	}

	// Sync point. Wait for sometime to let the subscriptions be registered.
	time.Sleep(time.Millisecond * 100)

	c.Check(queue.Report(), tc.DeepEquals, map[string]any{
		"subscriptions":        10,
		"subscriptions-by-ns":  1,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	for _, sub := range subs {
		s.unsubscribe(c, sub)
	}

	c.Check(queue.Report(), tc.DeepEquals, map[string]any{
		"subscriptions":        0,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestReportWithMultipleDuplicateTopicSubscriptions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAfter()
	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc().Times(10)
	s.metrics.EXPECT().SubscriptionsDec().Times(10)

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	var subs []changestream.Subscription
	for i := 0; i < 10; i++ {
		sub, err := queue.Subscribe(
			changestream.Namespace("topic", changestreamtesting.Create),
			changestream.Namespace("topic", changestreamtesting.Update),
		)
		c.Assert(err, tc.ErrorIsNil)

		subs = append(subs, sub)
	}

	// Sync point. Wait for sometime to let the subscriptions be registered.
	time.Sleep(time.Millisecond * 100)

	c.Check(queue.Report(), tc.DeepEquals, map[string]any{
		"subscriptions":        10,
		"subscriptions-by-ns":  1,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	for _, sub := range subs {
		s.unsubscribe(c, sub)
	}

	c.Check(queue.Report(), tc.DeepEquals, map[string]any{
		"subscriptions":        0,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestReportWithTopicRemovalAfterUnsubscribe(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAfter()
	s.expectStreamDying(make(<-chan struct{}))

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	s.metrics.EXPECT().SubscriptionsInc()
	s.metrics.EXPECT().SubscriptionsDec()

	queue, err := New(s.stream, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestreamtesting.Create))
	c.Assert(err, tc.ErrorIsNil)

	c.Check(queue.Report(), tc.DeepEquals, map[string]any{
		"subscriptions":        1,
		"subscriptions-by-ns":  1,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	s.unsubscribe(c, sub)

	c.Check(queue.Report(), tc.DeepEquals, map[string]any{
		"subscriptions":        0,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) unsubscribe(c *tc.C, sub changestream.Subscription) {
	sub.Unsubscribe()

	select {
	case <-sub.Done():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}
}
