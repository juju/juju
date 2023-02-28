// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventqueue

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/testing"
)

type eventQueueSuite struct {
	baseSuite
}

var _ = gc.Suite(&eventQueueSuite{})

func (s *eventQueueSuite) TestSubscribe(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).AnyTimes()

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(func(changestream.ChangeEvent) {
		c.Fatal("failed if called")
	}, changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)
	sub.Unsubscribe()

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestDispatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	done := make(chan struct{})
	sub, err := queue.Subscribe(func(event changestream.ChangeEvent) {
		defer close(done)

		c.Assert(event.Type(), jc.DeepEquals, changestream.Create)
		c.Assert(event.Namespace(), jc.DeepEquals, "topic")

	}, changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	s.expectChangeEvent(changestream.Create, "topic")

	go func() {
		select {
		case changes <- s.changeEvent:
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	sub.Unsubscribe()

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestMultipleDispatch(c *gc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestream.Update))
}

func (s *eventQueueSuite) TestMultipleDispatchWithMultipleMasks(c *gc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestream.Create|changestream.Update))
}

func (s *eventQueueSuite) TestMultipleDispatchWithMultipleOptions(c *gc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestream.Update), changestream.Namespace("topic", changestream.Create))
}

func (s *eventQueueSuite) TestMultipleDispatchWithOverlappingOptions(c *gc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestream.Update), changestream.Namespace("topic", changestream.Update|changestream.Create))
}

func (s *eventQueueSuite) TestSubscribeWithMatchingFilter(c *gc.C) {
	s.testMultipleDispatch(c, changestream.FilteredNamespace("topic", changestream.Update, func(event changestream.ChangeEvent) bool {
		return event.Namespace() == "topic"
	}))
}

func (s *eventQueueSuite) testMultipleDispatch(c *gc.C, opts ...changestream.SubscriptionOption) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	s.expectChangeEvent(changestream.Update, "topic")

	wg := newWaitGroup(10)
	subs := make([]changestream.Subscription, 10)
	for i := 0; i < len(subs); i++ {
		sub, err := queue.Subscribe(func(event changestream.ChangeEvent) {
			defer wg.Done()
			c.Assert(event.Type(), jc.DeepEquals, changestream.Update)
			c.Assert(event.Namespace(), jc.DeepEquals, "topic")

		}, opts...)
		c.Assert(err, jc.ErrorIsNil)

		subs[i] = sub
	}

	go func() {
		select {
		case changes <- s.changeEvent:
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()

	select {
	case <-wg.Wait():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	for _, sub := range subs {
		sub.Unsubscribe()
	}

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestUnsubscribeDuringDispatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	done := make(chan struct{})
	var (
		sub changestream.Subscription
		err error
	)
	sub, err = queue.Subscribe(func(event changestream.ChangeEvent) {
		defer close(done)

		sub.Unsubscribe()

	}, changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	s.expectChangeEvent(changestream.Create, "topic")

	go func() {
		select {
		case changes <- s.changeEvent:
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	sub.Unsubscribe()

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestUnsubscribeTwice(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(func(event changestream.ChangeEvent) {
	}, changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	sub.Unsubscribe()
	sub.Unsubscribe()

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestTopicDoesNotMatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	done := make(chan struct{})
	sub, err := queue.Subscribe(func(event changestream.ChangeEvent) {
		c.Fatal("failed if called")

	}, changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	s.changeEvent.EXPECT().Namespace().Return("foo").MinTimes(1)

	go func() {
		defer close(done)
		select {
		case changes <- s.changeEvent:
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	sub.Unsubscribe()

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestTopicMatchesOne(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	sub0, err := queue.Subscribe(func(event changestream.ChangeEvent) {
		c.Fatal("failed if called")
	}, changestream.Namespace("foo", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	done := make(chan struct{})
	sub1, err := queue.Subscribe(func(event changestream.ChangeEvent) {
		defer close(done)

		c.Assert(event.Type(), jc.DeepEquals, changestream.Create)
		c.Assert(event.Namespace(), jc.DeepEquals, "topic")
	}, changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	s.expectChangeEvent(changestream.Create, "topic")

	go func() {
		select {
		case changes <- s.changeEvent:
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	sub0.Unsubscribe()
	sub1.Unsubscribe()

	workertest.CleanKill(c, queue)
}
