// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/changestream"
	changestreamtesting "github.com/juju/juju/core/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type benchSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&benchSuite{})

type mockMetrics struct {
}

func (*mockMetrics) SubscriptionsInc()                                {}
func (*mockMetrics) SubscriptionsDec()                                {}
func (*mockMetrics) SubscriptionsClear()                              {}
func (*mockMetrics) DispatchDurationObserve(val float64, failed bool) {}
func (*mockMetrics) DispatchErrorsInc()                               {}

func benchmarkSignal(c *tc.C, changes ChangeSet) {
	sub := newSubscription(0, func() {})
	defer workertest.CleanKill(c, sub)

	ctx := context.Background()

	done := consume(c, sub)
	defer close(done)

	// Reset the timer so that we don't include the setup in the benchmark.
	c.ResetTimer()

	for i := 0; i < c.N; i++ {
		sub.dispatch(ctx, changes)
	}

	workertest.CleanKill(c, sub)
}

func create(size int) ChangeSet {
	changes := make(ChangeSet, size)
	for i := 0; i < size; i++ {
		changes[i] = &changeEvent{
			ctype:   changestreamtesting.Update,
			ns:      "test",
			changed: fmt.Sprintf("uuid-%d", i),
		}
	}
	return changes
}

func consume(c *tc.C, sub changestream.Subscription) chan<- struct{} {
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-sub.Changes():
			case <-done:
				return
			}
		}
	}()
	return done
}

func (s *benchSuite) BenchmarkSignal_1(c *tc.C) {
	benchmarkSignal(c, create(1))
}

func (s *benchSuite) BenchmarkSignal_10(c *tc.C) {
	benchmarkSignal(c, create(10))
}

func (s *benchSuite) BenchmarkSignal_100(c *tc.C) {
	benchmarkSignal(c, create(100))
}

func (s *benchSuite) BenchmarkSignal_1000(c *tc.C) {
	benchmarkSignal(c, create(1000))
}

func benchmarkSubscriptions(c *tc.C, numSubs, numEvents int, ns string) {
	terms := make(chan changestream.Term)

	em, err := New(stream{
		terms: terms,
	}, clock.WallClock, &mockMetrics{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.IsNil)
	defer workertest.CleanKill(c, em)

	completed := make([]chan<- struct{}, numSubs)
	for i := 0; i < numSubs; i++ {
		sub, err := em.Subscribe(changestream.Namespace(ns, changestreamtesting.Update))
		c.Assert(err, tc.IsNil)

		done := consume(c, sub)
		completed = append(completed, done)
	}

	// This will close with the benchmark is done, not when the for loop
	// exits.
	defer func() {
		for _, ch := range completed {
			close(ch)
		}
	}()

	changes := create(numEvents)

	c.ResetTimer()

	for i := 0; i < c.N; i++ {
		terms <- term{changes: changes}
	}

	workertest.CleanKill(c, em)
}

func (s *benchSuite) BenchmarkMatching_1_1(c *tc.C) {
	benchmarkSubscriptions(c, 1, 1, "test")
}

func (s *benchSuite) BenchmarkMatching_1_10(c *tc.C) {
	benchmarkSubscriptions(c, 1, 10, "test")
}

func (s *benchSuite) BenchmarkMatching_1_100(c *tc.C) {
	benchmarkSubscriptions(c, 1, 100, "test")
}

func (s *benchSuite) BenchmarkMatching_10_1(c *tc.C) {
	benchmarkSubscriptions(c, 10, 1, "test")
}

func (s *benchSuite) BenchmarkMatching_10_10(c *tc.C) {
	benchmarkSubscriptions(c, 10, 10, "test")
}

func (s *benchSuite) BenchmarkMatching_10_100(c *tc.C) {
	benchmarkSubscriptions(c, 10, 100, "test")
}

func (s *benchSuite) BenchmarkMatching_100_1(c *tc.C) {
	benchmarkSubscriptions(c, 100, 1, "test")
}

func (s *benchSuite) BenchmarkMatching_100_10(c *tc.C) {
	benchmarkSubscriptions(c, 100, 10, "test")
}

func (s *benchSuite) BenchmarkMatching_100_100(c *tc.C) {
	benchmarkSubscriptions(c, 100, 100, "test")
}

func (s *benchSuite) BenchmarkMatching_1000_1(c *tc.C) {
	benchmarkSubscriptions(c, 1000, 1, "test")
}

func (s *benchSuite) BenchmarkMatching_1000_10(c *tc.C) {
	benchmarkSubscriptions(c, 1000, 10, "test")
}

func (s *benchSuite) BenchmarkMatching_1000_100(c *tc.C) {
	benchmarkSubscriptions(c, 1000, 100, "test")
}

func (s *benchSuite) BenchmarkNonMatching_1_1(c *tc.C) {
	benchmarkSubscriptions(c, 1, 1, "bar")
}

func (s *benchSuite) BenchmarkNonMatching_1_10(c *tc.C) {
	benchmarkSubscriptions(c, 1, 10, "bar")
}

func (s *benchSuite) BenchmarkNonMatching_1_100(c *tc.C) {
	benchmarkSubscriptions(c, 1, 100, "bar")
}

func (s *benchSuite) BenchmarkNonMatching_10_1(c *tc.C) {
	benchmarkSubscriptions(c, 10, 1, "bar")
}

func (s *benchSuite) BenchmarkNonMatching_10_10(c *tc.C) {
	benchmarkSubscriptions(c, 10, 10, "bar")
}

func (s *benchSuite) BenchmarkNonMatching_10_100(c *tc.C) {
	benchmarkSubscriptions(c, 10, 100, "bar")
}

func (s *benchSuite) BenchmarkNonMatching_100_1(c *tc.C) {
	benchmarkSubscriptions(c, 100, 1, "bar")
}

func (s *benchSuite) BenchmarkNonMatching_100_10(c *tc.C) {
	benchmarkSubscriptions(c, 100, 10, "bar")
}

func (s *benchSuite) BenchmarkNonMatching_100_100(c *tc.C) {
	benchmarkSubscriptions(c, 100, 100, "bar")
}

func (s *benchSuite) BenchmarkNonMatching_1000_1(c *tc.C) {
	benchmarkSubscriptions(c, 1000, 1, "bar")
}

func (s *benchSuite) BenchmarkNonMatching_1000_10(c *tc.C) {
	benchmarkSubscriptions(c, 1000, 10, "bar")
}

func (s *benchSuite) BenchmarkNonMatching_1000_100(c *tc.C) {
	benchmarkSubscriptions(c, 1000, 100, "bar")
}

type stream struct {
	terms chan changestream.Term
	dying chan struct{}
}

func (s stream) Terms() <-chan changestream.Term {
	return s.terms
}

func (s stream) Dying() <-chan struct{} {
	return s.dying
}

type term struct {
	changes ChangeSet
}

func (t term) Changes() ChangeSet {
	return t.changes
}

func (t term) Done(empty bool, abort <-chan struct{}) {}
