// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	time "time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	changestream "github.com/juju/juju/core/changestream"
	"github.com/juju/juju/testing"
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

var _ = gc.Suite(&eventMultiplexerSuite{})

func (s *eventMultiplexerSuite) TestSubscribe(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).AnyTimes()

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	s.unsubscribe(c, sub)

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestDispatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	s.expectTerm(c, changeEvent{
		ctype: changestream.Create,
		ns:    "topic",
		uuid:  "1",
	})
	s.dispatchTerm(c, terms)

	var changes []changestream.ChangeEvent
	select {
	case changes = <-sub.Changes():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	c.Assert(changes, gc.HasLen, 1)
	c.Check(changes[0].Type(), jc.DeepEquals, changestream.Create)
	c.Check(changes[0].Namespace(), jc.DeepEquals, "topic")
	c.Check(changes[0].ChangedUUID(), gc.Equals, "1")

	s.unsubscribe(c, sub)

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestMultipleDispatch(c *gc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestream.Update))
}

func (s *eventMultiplexerSuite) TestDispatchWithNoOptions(c *gc.C) {
	s.testMultipleDispatch(c)
}

func (s *eventMultiplexerSuite) TestMultipleDispatchWithMultipleMasks(c *gc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestream.Create|changestream.Update))
}

func (s *eventMultiplexerSuite) TestMultipleDispatchWithMultipleOptions(c *gc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestream.Update), changestream.Namespace("topic", changestream.Create))
}

func (s *eventMultiplexerSuite) TestMultipleDispatchWithOverlappingOptions(c *gc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestream.Update), changestream.Namespace("topic", changestream.Update|changestream.Create))
}

func (s *eventMultiplexerSuite) TestMultipleDispatchWithDuplicateOptions(c *gc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestream.Update), changestream.Namespace("topic", changestream.Update))
}

func (s *eventMultiplexerSuite) TestSubscribeWithMatchingFilter(c *gc.C) {
	s.testMultipleDispatch(c, changestream.FilteredNamespace("topic", changestream.Update, func(event changestream.ChangeEvent) bool {
		return event.Namespace() == "topic"
	}))
}

func (s *eventMultiplexerSuite) testMultipleDispatch(c *gc.C, opts ...changestream.SubscriptionOption) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	s.expectTerm(c, changeEvent{
		ctype: changestream.Update,
		ns:    "topic",
		uuid:  "1",
	})

	subs := make([]changestream.Subscription, 10)
	for i := 0; i < len(subs); i++ {
		sub, err := queue.Subscribe(opts...)
		c.Assert(err, jc.ErrorIsNil)

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
				c.Assert(events, gc.HasLen, 1)
				c.Check(events[0].Type(), jc.DeepEquals, changestream.Update)
				c.Check(events[0].Namespace(), jc.DeepEquals, "topic")
			case <-time.After(testing.ShortWait):
				c.Fatalf("timed out waiting for sub %d event", i)
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

func (s *eventMultiplexerSuite) TestUnsubscribeTwice(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	s.expectTerm(c, changeEvent{
		ctype: changestream.Create,
		ns:    "topic",
		uuid:  "1",
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

func (s *eventMultiplexerSuite) TestTopicDoesNotMatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	s.expectEmptyTerm(c, changeEvent{
		ctype: changestream.Create,
		ns:    "foo",
		uuid:  "1",
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

func (s *eventMultiplexerSuite) TestTopicMatchesOne(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	sub0, err := queue.Subscribe(changestream.Namespace("foo", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	sub1, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	s.expectTerm(c, changeEvent{
		ctype: changestream.Create,
		ns:    "topic",
		uuid:  "1",
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

func (s *eventMultiplexerSuite) TestSubscriptionDoneWhenEventQueueKilled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	s.expectTerm(c, changeEvent{
		ctype: changestream.Create,
		ns:    "topic",
		uuid:  "1",
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

func (s *eventMultiplexerSuite) TestUnsubscribeOfOtherSubscription(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	subs := make([]changestream.Subscription, 2)
	for i := 0; i < len(subs); i++ {
		sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
		c.Assert(err, jc.ErrorIsNil)
		subs[i] = sub
	}

	s.expectTerm(c, changeEvent{
		ctype: changestream.Create,
		ns:    "topic",
		uuid:  "1",
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
			case <-time.After(testing.ShortWait):
				c.Fatalf("timed out waiting for sub %d event", i)
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

func (s *eventMultiplexerSuite) TestUnsubscribeOfOtherSubscriptionInAnotherGoroutine(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	subs := make([]changestream.Subscription, 2)
	for i := 0; i < len(subs); i++ {

		sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
		c.Assert(err, jc.ErrorIsNil)
		subs[i] = sub
	}

	s.expectTerm(c, changeEvent{
		ctype: changestream.Create,
		ns:    "topic",
		uuid:  "1",
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
			case <-time.After(testing.ShortWait):
				c.Fatalf("timed out waiting for sub %d event", i)
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

func (s *eventMultiplexerSuite) TestReportWithAllSubscriptions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectAfter()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	var subs []changestream.Subscription
	for i := 0; i < 10; i++ {
		sub, err := queue.Subscribe()
		c.Assert(err, jc.ErrorIsNil)

		subs = append(subs, sub)
	}

	// Sync point. Wait for sometime to let the subscriptions be registered.
	time.Sleep(time.Millisecond * 100)

	c.Check(queue.Report(), gc.DeepEquals, map[string]any{
		"subscriptions":        10,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    10,
		"dispatch-error-count": 0,
	})

	for _, sub := range subs {
		s.unsubscribe(c, sub)
	}

	c.Check(queue.Report(), gc.DeepEquals, map[string]any{
		"subscriptions":        0,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestReportWithTopicSubscriptions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectAfter()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	var subs []changestream.Subscription
	for i := 0; i < 10; i++ {
		sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
		c.Assert(err, jc.ErrorIsNil)

		subs = append(subs, sub)
	}

	// Sync point. Wait for sometime to let the subscriptions be registered.
	time.Sleep(time.Millisecond * 100)

	c.Check(queue.Report(), gc.DeepEquals, map[string]any{
		"subscriptions":        10,
		"subscriptions-by-ns":  1,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	for _, sub := range subs {
		s.unsubscribe(c, sub)
	}

	c.Check(queue.Report(), gc.DeepEquals, map[string]any{
		"subscriptions":        0,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestReportWithMultipleTopicSubscriptions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectAfter()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	var subs []changestream.Subscription
	for i := 0; i < 10; i++ {
		sub, err := queue.Subscribe(
			changestream.Namespace("topic", changestream.Create),
			changestream.Namespace("foo", changestream.Update),
		)
		c.Assert(err, jc.ErrorIsNil)

		subs = append(subs, sub)
	}

	// Sync point. Wait for sometime to let the subscriptions be registered.
	time.Sleep(time.Millisecond * 100)

	c.Check(queue.Report(), gc.DeepEquals, map[string]any{
		"subscriptions":        10,
		"subscriptions-by-ns":  2,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	for _, sub := range subs {
		s.unsubscribe(c, sub)
	}

	c.Check(queue.Report(), gc.DeepEquals, map[string]any{
		"subscriptions":        0,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestReportWithDuplicateTopicSubscriptions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectAfter()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	var subs []changestream.Subscription
	for i := 0; i < 10; i++ {
		sub, err := queue.Subscribe(
			changestream.Namespace("topic", changestream.Update),
			changestream.Namespace("topic", changestream.Update),
		)
		c.Assert(err, jc.ErrorIsNil)

		subs = append(subs, sub)
	}

	// Sync point. Wait for sometime to let the subscriptions be registered.
	time.Sleep(time.Millisecond * 100)

	c.Check(queue.Report(), gc.DeepEquals, map[string]any{
		"subscriptions":        10,
		"subscriptions-by-ns":  1,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	for _, sub := range subs {
		s.unsubscribe(c, sub)
	}

	c.Check(queue.Report(), gc.DeepEquals, map[string]any{
		"subscriptions":        0,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestReportWithMultipleDuplicateTopicSubscriptions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectAfter()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	var subs []changestream.Subscription
	for i := 0; i < 10; i++ {
		sub, err := queue.Subscribe(
			changestream.Namespace("topic", changestream.Create),
			changestream.Namespace("topic", changestream.Update),
		)
		c.Assert(err, jc.ErrorIsNil)

		subs = append(subs, sub)
	}

	// Sync point. Wait for sometime to let the subscriptions be registered.
	time.Sleep(time.Millisecond * 100)

	c.Check(queue.Report(), gc.DeepEquals, map[string]any{
		"subscriptions":        10,
		"subscriptions-by-ns":  1,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	for _, sub := range subs {
		s.unsubscribe(c, sub)
	}

	c.Check(queue.Report(), gc.DeepEquals, map[string]any{
		"subscriptions":        0,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) TestReportWithTopicRemovalAfterUnsubscribe(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectAfter()

	terms := make(chan changestream.Term)
	s.stream.EXPECT().Terms().Return(terms).MinTimes(1)

	queue, err := New(s.stream, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(queue.Report(), gc.DeepEquals, map[string]any{
		"subscriptions":        1,
		"subscriptions-by-ns":  1,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	s.unsubscribe(c, sub)

	c.Check(queue.Report(), gc.DeepEquals, map[string]any{
		"subscriptions":        0,
		"subscriptions-by-ns":  0,
		"subscriptions-all":    0,
		"dispatch-error-count": 0,
	})

	workertest.CleanKill(c, queue)
}

func (s *eventMultiplexerSuite) unsubscribe(c *gc.C, sub changestream.Subscription) {
	sub.Unsubscribe()

	select {
	case <-sub.Done():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}
}
