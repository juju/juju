// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"fmt"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/changestream"
	changestreamtesting "github.com/juju/juju/core/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type mockMetrics struct {
}

func (*mockMetrics) SubscriptionsInc()                                {}
func (*mockMetrics) SubscriptionsDec()                                {}
func (*mockMetrics) SubscriptionsClear()                              {}
func (*mockMetrics) DispatchDurationObserve(val float64, failed bool) {}
func (*mockMetrics) DispatchErrorsInc()                               {}

func benchmarkSignal(b *testing.B, changes ChangeSet) {
	c := &tc.TBC{TB: b}
	sub := newSubscription(0, func() {})
	defer workertest.CleanKill(c, sub)

	ctx := c.Context()

	done := consume(b, sub)
	defer close(done)

	// Reset the timer so that we don't include the setup in the benchmark.
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
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

func consume(b *testing.B, sub changestream.Subscription) chan<- struct{} {
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

func BenchmarkSignal_1(b *testing.B) {
	benchmarkSignal(b, create(1))
}

func BenchmarkSignal_10(b *testing.B) {
	benchmarkSignal(b, create(10))
}

func BenchmarkSignal_100(b *testing.B) {
	benchmarkSignal(b, create(100))
}

func BenchmarkSignal_1000(b *testing.B) {
	benchmarkSignal(b, create(1000))
}

func benchmarkSubscriptions(b *testing.B, numSubs, numEvents int, ns string) {
	c := &tc.TBC{TB: b}
	terms := make(chan changestream.Term)

	em, err := New(stream{
		terms: terms,
	}, clock.WallClock, &mockMetrics{}, loggertesting.WrapCheckLog(b))
	tc.Assert(b, err, tc.IsNil)
	defer workertest.CleanKill(c, em)

	completed := make([]chan<- struct{}, 0, numSubs)
	for i := 0; i < numSubs; i++ {
		sub, err := em.Subscribe(changestream.Namespace(ns, changestreamtesting.Update))
		c.Assert(err, tc.IsNil)

		done := consume(b, sub)
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

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		terms <- term{changes: changes}
	}

	workertest.CleanKill(c, em)
}

func BenchmarkMatching_1_1(b *testing.B) {
	benchmarkSubscriptions(b, 1, 1, "test")
}

func BenchmarkMatching_1_10(b *testing.B) {
	benchmarkSubscriptions(b, 1, 10, "test")
}

func BenchmarkMatching_1_100(b *testing.B) {
	benchmarkSubscriptions(b, 1, 100, "test")
}

func BenchmarkMatching_10_1(b *testing.B) {
	benchmarkSubscriptions(b, 10, 1, "test")
}

func BenchmarkMatching_10_10(b *testing.B) {
	benchmarkSubscriptions(b, 10, 10, "test")
}

func BenchmarkMatching_10_100(b *testing.B) {
	benchmarkSubscriptions(b, 10, 100, "test")
}

func BenchmarkMatching_100_1(b *testing.B) {
	benchmarkSubscriptions(b, 100, 1, "test")
}

func BenchmarkMatching_100_10(b *testing.B) {
	benchmarkSubscriptions(b, 100, 10, "test")
}

func BenchmarkMatching_100_100(b *testing.B) {
	benchmarkSubscriptions(b, 100, 100, "test")
}

func BenchmarkMatching_1000_1(b *testing.B) {
	benchmarkSubscriptions(b, 1000, 1, "test")
}

func BenchmarkMatching_1000_10(b *testing.B) {
	benchmarkSubscriptions(b, 1000, 10, "test")
}

func BenchmarkMatching_1000_100(b *testing.B) {
	benchmarkSubscriptions(b, 1000, 100, "test")
}

func BenchmarkNonMatching_1_1(b *testing.B) {
	benchmarkSubscriptions(b, 1, 1, "bar")
}

func BenchmarkNonMatching_1_10(b *testing.B) {
	benchmarkSubscriptions(b, 1, 10, "bar")
}

func BenchmarkNonMatching_1_100(b *testing.B) {
	benchmarkSubscriptions(b, 1, 100, "bar")
}

func BenchmarkNonMatching_10_1(b *testing.B) {
	benchmarkSubscriptions(b, 10, 1, "bar")
}

func BenchmarkNonMatching_10_10(b *testing.B) {
	benchmarkSubscriptions(b, 10, 10, "bar")
}

func BenchmarkNonMatching_10_100(b *testing.B) {
	benchmarkSubscriptions(b, 10, 100, "bar")
}

func BenchmarkNonMatching_100_1(b *testing.B) {
	benchmarkSubscriptions(b, 100, 1, "bar")
}

func BenchmarkNonMatching_100_10(b *testing.B) {
	benchmarkSubscriptions(b, 100, 10, "bar")
}

func BenchmarkNonMatching_100_100(b *testing.B) {
	benchmarkSubscriptions(b, 100, 100, "bar")
}

func BenchmarkNonMatching_1000_1(b *testing.B) {
	benchmarkSubscriptions(b, 1000, 1, "bar")
}

func BenchmarkNonMatching_1000_10(b *testing.B) {
	benchmarkSubscriptions(b, 1000, 10, "bar")
}

func BenchmarkNonMatching_1000_100(b *testing.B) {
	benchmarkSubscriptions(b, 1000, 100, "bar")
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
