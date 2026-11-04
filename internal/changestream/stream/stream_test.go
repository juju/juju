// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stream

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	stdtesting "testing"
	"time"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/changestream"
	changestreamtesting "github.com/juju/juju/core/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

const (
	// We need to ensure that we never witness a change event. We've picked
	// an arbitrary timeout of 1 second, which should be more than enough
	// time for the worker to process the change.
	witnessChangeLongDuration  = time.Second
	witnessChangeShortDuration = witnessChangeLongDuration / 2
)

type streamSuite struct {
	baseSuite
}

func TestStreamSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &streamSuite{})
	})
}

func (s *streamSuite) TestWithNoNamespace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFileNotifyWatcher()
	s.expectAfter()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	select {
	case <-stream.Terms():
		c.Fatal("timed out waiting for term")
	case <-time.After(witnessChangeLongDuration):
	}

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestNoData(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFileNotifyWatcher()
	s.expectAfter()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	select {
	case <-stream.Terms():
		c.Fatal("timed out waiting for term")
	case <-time.After(witnessChangeLongDuration):
	}

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestOneChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFileNotifyWatcher()
	s.expectAnyAfterAnyTimes()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	chg := change{
		id:   1000,
		uuid: uuid.MustNewUUID().String(),
	}
	s.insertChange(c, chg)

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	var results []changestream.ChangeEvent
	select {
	case term := <-stream.Terms():
		results = term.Changes()
		term.Done(false, make(chan struct{}))

	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	expectChanges(c, []change{chg}, results)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestOneChangeDoesNotRepeatSameChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)

	s.expectFileNotifyWatcher()
	s.expectTermAfterAnyTimes()
	s.expectBackoffAnyTimes(done)
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	chg := change{
		id:   1000,
		uuid: uuid.MustNewUUID().String(),
	}
	s.insertChange(c, chg)

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	var results []changestream.ChangeEvent
	select {
	case term := <-stream.Terms():
		results = term.Changes()
		term.Done(false, make(chan struct{}))

	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	expectChanges(c, []change{chg}, results)

	chg = change{
		id:   1000,
		uuid: uuid.MustNewUUID().String(),
	}
	s.insertChange(c, chg)

	select {
	case term := <-stream.Terms():
		results = term.Changes()
		term.Done(false, make(chan struct{}))

	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	expectChanges(c, []change{chg}, results)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestOneChangeWithEmptyResults(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)

	s.expectFileNotifyWatcher()
	s.expectTermAfterAnyTimes()
	s.expectBackoffAnyTimes(done)
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	chg := change{
		id:   1000,
		uuid: uuid.MustNewUUID().String(),
	}
	s.insertChange(c, chg)

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	var results []changestream.ChangeEvent
	select {
	case term := <-stream.Terms():
		results = term.Changes()
		term.Done(true, make(chan struct{}))

	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	expectChanges(c, []change{chg}, results)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestOneChangeWithClosedAbort(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFileNotifyWatcher()
	s.expectAfterWithoutTermTimeout()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	chg := change{
		id:   1000,
		uuid: uuid.MustNewUUID().String(),
	}
	s.insertChange(c, chg)

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	var results []changestream.ChangeEvent
	select {
	case term := <-stream.Terms():
		results = term.Changes()

		// Force the close of the abort channel.
		ch := make(chan struct{})
		close(ch)
		term.Done(false, ch)

	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	expectChanges(c, []change{chg}, results)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestOneChangeWithDelayedTermDone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFileNotifyWatcher()
	// Handles both term-timeout and backoff After calls to prevent
	// deadlock in the backoff path after term.Done completes.
	s.expectAfterWithoutTermTimeout()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	chg := change{
		id:   1000,
		uuid: uuid.MustNewUUID().String(),
	}
	s.insertChange(c, chg)

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	var (
		results []changestream.ChangeEvent
		term    changestream.Term
	)
	select {
	case term = <-stream.Terms():
		results = term.Changes()

	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	expectChanges(c, []change{chg}, results)

	term.Done(false, make(chan struct{}))

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestOneChangeWithTermDoneAfterKill(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFileNotifyWatcher()
	s.expectTermAfterAnyTimes()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	chg := change{
		id:   1000,
		uuid: uuid.MustNewUUID().String(),
	}
	s.insertChange(c, chg)

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	var (
		results []changestream.ChangeEvent
		term    changestream.Term
	)
	select {
	case term = <-stream.Terms():
		results = term.Changes()

	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	expectChanges(c, []change{chg}, results)

	workertest.CleanKill(c, stream)

	// Ensure that we don't panic after the stream has been killed.
	ch := make(chan struct{})
	close(ch)
	term.Done(false, ch)
}

func (s *streamSuite) TestOneChangeWithTimeoutCausesWorkerToBounce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFileNotifyWatcher()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now().UTC()
		return ch
	}).AnyTimes()

	s.insertNamespace(c, 1000, "foo")

	chg := change{
		id:   1000,
		uuid: uuid.MustNewUUID().String(),
	}
	s.insertChange(c, chg)

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	select {
	case <-stream.Terms():
		// Normally we'd call term.Done() here, but we want to ensure that
		// the worker is bounced, so we'll just let the term timeout.
		<-time.After(witnessChangeShortDuration)

	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	err := workertest.CheckKill(c, stream)
	c.Assert(err, tc.ErrorMatches, `term has not been completed in time`)
}

func (s *streamSuite) TestMultipleTerms(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)

	s.expectFileNotifyWatcher()
	s.expectTermAfterAnyTimes()
	s.expectBackoffAnyTimes(done)
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	for range 10 {
		// Insert a change and wait for it to be streamed.
		chg := change{
			id:   1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, chg)

		var (
			results []changestream.ChangeEvent
			term    changestream.Term
		)
		select {
		case term = <-stream.Terms():
			results = term.Changes()
			term.Done(false, make(chan struct{}))

		case <-c.Context().Done():
			c.Fatal("timed out waiting for change")
		}

		expectChanges(c, []change{chg}, results)
	}

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestMultipleTermsAllEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFileNotifyWatcher()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	done := make(chan struct{})
	defer close(done)

	var duration int64
	s.clock.EXPECT().After(defaultWaitTermTimeout).Return(make(chan time.Time)).AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(d time.Duration) <-chan time.Time {
		if atomic.LoadInt64(&duration) > d.Microseconds() {
			c.Fatalf("expected duration %d to be greater than %d", d.Microseconds(), atomic.LoadInt64(&duration))
		}
		atomic.SwapInt64(&duration, d.Microseconds())

		ch := make(chan time.Time)
		go func() {
			select {
			case ch <- time.Now().UTC():
			case <-done:
			}
		}()
		return ch
	}).AnyTimes()

	s.insertNamespace(c, 1000, "foo")

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	for range 10 {
		// Insert a change and wait for it to be streamed.
		chg := change{
			id:   1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, chg)

		var (
			results []changestream.ChangeEvent
			term    changestream.Term
		)
		select {
		case term = <-stream.Terms():
			results = term.Changes()
			term.Done(true, make(chan struct{}))

		case <-c.Context().Done():
			c.Fatal("timed out waiting for change")
		}

		expectChanges(c, []change{chg}, results)
	}

	workertest.CleanKill(c, stream)
}

// Ensure that we don't attempt to read any more terms until after the first
// term has been done.
func (s *streamSuite) TestSecondTermDoesNotStartUntilFirstTermDone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFileNotifyWatcher()
	s.expectAfterWithoutTermTimeout()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	statesChan := make(chan []string, 1)
	noTermDeadline := time.Time{}
	stream := NewInternalStates(uuid.MustNewUUID().String(),
		s.TxnRunner(),
		s.FileNotifier,
		s.clock,
		s.metrics,
		loggertesting.WrapCheckLog(c),
		noTermDeadline,
		statesChan)
	defer workertest.DirtyKill(c, stream)

	// Ensure the stream has started.
	select {
	case states := <-statesChan:
		if !slices.Contains(states, stateBegin) {
			c.Fatal("missing begin state")
		}
	case <-c.Context().Done():
		c.Fatal("timed out waiting for begin state")
	}

	// Insert a change and wait for it to be streamed.
	chg := change{
		id:   1000,
		uuid: uuid.MustNewUUID().String(),
	}
	s.insertChange(c, chg)

	var (
		results []changestream.ChangeEvent
		term    changestream.Term
	)
	select {
	case term = <-stream.Terms():
		results = term.Changes()

	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	expectChanges(c, []change{chg}, results)

	// We should never witness the following change until the term is done.
	chg = change{
		id:   1000,
		uuid: uuid.MustNewUUID().String(),
	}
	s.insertChange(c, chg)

	// Wait to ensure that the loop has been given enough time to read the
	// changes. Even though know we're blocked on the term.Done() call below,
	// we still need to wait for the possibility the loop to read the change.
	<-time.After(witnessChangeLongDuration)

	// Now attempt to locate the second change, even though it should always
	// fail.
	select {
	case <-stream.Terms():
		c.Fatal("unexpected term")
	case <-time.After(witnessChangeShortDuration):
	}

	// Finish the term.
	term.Done(false, make(chan struct{}))

	// Wait to ensure that the loop has been given enough time to read the
	// changes.
	<-time.After(witnessChangeShortDuration)

	// Now the term is done, we should be able to witness the second change.
	select {
	case term = <-stream.Terms():
		results = term.Changes()

	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	expectChanges(c, []change{chg}, results)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestMultipleChangesWithSameUUIDCoalesce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectTermAfterAnyTimes()
	s.expectFileNotifyWatcher()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	var inserts []change
	for range 4 {
		ch := change{
			id:   1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	// Force a coalesce change through, we should not see three changes, instead
	// we should just see one.
	for range 2 {
		s.insertChange(c, inserts[len(inserts)-1])
	}

	for range 4 {
		ch := change{
			id:   1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	// Wait to ensure that the loop has been given enough time to read the
	// changes.
	<-time.After(witnessChangeShortDuration)

	var results []changestream.ChangeEvent
	select {
	case term := <-stream.Terms():
		results = append(results, term.Changes()...)
	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	c.Assert(results, tc.HasLen, 8)
	for i, result := range results {
		c.Check(result.Namespace(), tc.Equals, "foo")
		c.Check(result.Changed(), tc.Equals, inserts[i].uuid)
	}

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestMultipleChangesWithNamespaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectTermAfterAnyTimes()
	s.expectFileNotifyWatcher()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")
	s.insertNamespace(c, 2000, "bar")

	var inserts []change
	for i := range 10 {
		ch := change{
			id:   ((i % 2) + 1) * 1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	// Wait to ensure that the loop has been given enough time to read the
	// changes.
	<-time.After(witnessChangeShortDuration)

	var results []changestream.ChangeEvent
	select {
	case term := <-stream.Terms():
		results = append(results, term.Changes()...)
	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	c.Assert(results, tc.HasLen, 10)
	for i, result := range results {
		namespace := "foo"
		if inserts[i].id == 2000 {
			namespace = "bar"
		}
		c.Check(result.Namespace(), tc.Equals, namespace)
		c.Check(result.Changed(), tc.Equals, inserts[i].uuid)
	}

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestMultipleChangesWithNamespacesCoalesce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectTermAfterAnyTimes()
	s.expectFileNotifyWatcher()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")
	s.insertNamespace(c, 2000, "bar")

	var inserts []change
	for i := range 4 {
		ch := change{
			id:   ((i % 2) + 1) * 1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	// Force a coalesce change through, we should not see three changes, instead
	// we should just see one.
	for range 2 {
		s.insertChange(c, inserts[len(inserts)-1])
	}

	for i := range 4 {
		ch := change{
			id:   ((i % 2) + 1) * 1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	// Wait to ensure that the loop has been given enough time to read the
	// changes.
	<-time.After(witnessChangeShortDuration)

	var results []changestream.ChangeEvent
	select {
	case term := <-stream.Terms():
		results = append(results, term.Changes()...)
	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	c.Assert(results, tc.HasLen, 8)
	for i, result := range results {
		namespace := "foo"
		if inserts[i].id == 2000 {
			namespace = "bar"
		}
		c.Check(result.Namespace(), tc.Equals, namespace)
		c.Check(result.Changed(), tc.Equals, inserts[i].uuid)
	}

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestMultipleChangesWithNoNamespacesDoNotCoalesce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectTermAfterAnyTimes()
	s.expectFileNotifyWatcher()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")
	s.insertNamespace(c, 2000, "bar")
	s.insertNamespace(c, 3000, "baz")

	var inserts []change
	for i := range 4 {
		ch := change{
			id:   ((i % 2) + 1) * 1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	// Force a non coalesced change through. It has the same UUID, but not
	// the same namespace, so should come through as another change.
	ch := change{
		id:   3000,
		uuid: inserts[len(inserts)-1].uuid,
	}
	s.insertChange(c, ch)
	inserts = append(inserts, ch)

	// Force a coalesced change through. It has the same UUID and namespace,
	// so we should only see one change.
	s.insertChange(c, inserts[len(inserts)-1])

	for i := range 4 {
		ch := change{
			id:   ((i % 2) + 1) * 1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	// Wait to ensure that the loop has been given enough time to read the
	// changes.
	<-time.After(witnessChangeShortDuration)

	var results []changestream.ChangeEvent
	select {
	case term := <-stream.Terms():
		results = append(results, term.Changes()...)
	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	c.Assert(results, tc.HasLen, 9)
	for i, result := range results {
		namespace := "foo"
		if inserts[i].id == 2000 {
			namespace = "bar"
		} else if inserts[i].id == 3000 {
			namespace = "baz"
		}
		c.Check(result.Namespace(), tc.Equals, namespace)
		c.Check(result.Changed(), tc.Equals, inserts[i].uuid)
	}

	workertest.CleanKill(c, stream)
}

// TestLatestRowOfEachChangeAcrossTerms checks that the events of each term are
// built from the latest row of each namespace and changed value above the
// high-water mark left by the previous term. In each group of several rows the
// latest row has neither the latest created_at nor the first id, so an event
// built from another row of the group differs from the expected one.
func (s *streamSuite) TestLatestRowOfEachChangeAcrossTerms(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)

	s.expectFileNotifyWatcher()
	s.expectTermAfterAnyTimes()
	s.expectBackoffAnyTimes(done)
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")
	s.insertNamespace(c, 2000, "bar")

	// foo/k1 is created, updated and deleted, and the delete is the latest
	// row although it has an earlier created_at than the update. bar/k1 has
	// the same changed value in another namespace.
	ids := s.insertChangeRows(c,
		changeRow{changeType: changestreamtesting.Create, namespace: 1000, changed: "k1", createdAt: "2026-01-02 10:00:01.000"},
		changeRow{changeType: changestreamtesting.Update, namespace: 1000, changed: "k1", createdAt: "2026-01-02 10:00:05.000"},
		changeRow{changeType: changestreamtesting.Create, namespace: 2000, changed: "k1", createdAt: "2026-01-02 10:00:03.000"},
		changeRow{changeType: changestreamtesting.Delete, namespace: 1000, changed: "k1", createdAt: "2026-01-02 10:00:02.000"},
	)

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	select {
	case term := <-stream.Terms():
		s.expectEvents(c, ids, termEvents(c, term), []expectedEvent{
			{row: 2, changeType: changestreamtesting.Create, namespace: "bar", changed: "k1", createdAt: "2026-01-02 10:00:03.000"},
			{row: 3, changeType: changestreamtesting.Delete, namespace: "foo", changed: "k1", createdAt: "2026-01-02 10:00:02.000"},
		})
		term.Done(false, make(chan struct{}))

	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	// The first term has been completed, so the changes it contained must not
	// be seen again. The second term must contain just the latest row of
	// each change since then. foo/k1 ends with an update that was created
	// before the create preceding it and bar/k1 ends with a delete that has
	// the earliest created_at.
	// The rows are inserted in one transaction, so that they are all part of
	// the same term.
	second := s.insertChangeRows(c,
		changeRow{changeType: changestreamtesting.Create, namespace: 1000, changed: "k1", createdAt: "2026-01-02 10:00:09.000"},
		changeRow{changeType: changestreamtesting.Update, namespace: 1000, changed: "k1", createdAt: "2026-01-02 10:00:07.000"},
		changeRow{changeType: changestreamtesting.Update, namespace: 2000, changed: "k1", createdAt: "2026-01-02 10:00:08.000"},
		changeRow{changeType: changestreamtesting.Delete, namespace: 2000, changed: "k1", createdAt: "2026-01-02 10:00:06.000"},
	)
	ids = append(ids, second...)

	select {
	case term := <-stream.Terms():
		s.expectEvents(c, ids, termEvents(c, term), []expectedEvent{
			{row: 5, changeType: changestreamtesting.Update, namespace: "foo", changed: "k1", createdAt: "2026-01-02 10:00:07.000"},
			{row: 7, changeType: changestreamtesting.Delete, namespace: "bar", changed: "k1", createdAt: "2026-01-02 10:00:06.000"},
		})
		term.Done(false, make(chan struct{}))

	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	workertest.CleanKill(c, stream)
}

// TestConcurrentWritesAreStreamedAsLatestRows runs the stream while another
// goroutine appends batches of changes to a handful of namespace and changed
// values, and checks what every term, and the sequence of terms, must satisfy
// however the writes and the polls interleave:
//   - every event is the complete row of its id in the change log;
//   - a term is ordered by id and has one event for each namespace and changed
//     value;
//   - a term only contains rows above the highest id of the previous term;
//   - the final event of each namespace and changed value is its latest row,
//     so coalescing never loses the final state.
func (s *streamSuite) TestConcurrentWritesAreStreamedAsLatestRows(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)

	s.expectFileNotifyWatcher()
	s.expectTermAfterAnyTimes()
	s.expectBackoffAnyTimes(done)
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")
	s.insertNamespace(c, 2000, "bar")

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	// The writer sets writeErr and finalID before it closes writerDone, and
	// they are only read after writerDone has been closed.
	var (
		writerDone = make(chan struct{})
		writeErr   error
		finalID    int64
	)
	go func() {
		defer close(writerDone)

		ctx := c.Context()
		start := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)

		// Spread the created_at values out of order with respect to the id,
		// with no two rows sharing one.
		var n int
		for batch := range 40 {
			rows := make([]changeRow, 1+batch%5)
			for i := range rows {
				offset := time.Duration(n*7919%86400) * time.Second
				rows[i] = changeRow{
					changeType: changestream.ChangeType(1 << ((n + batch) % 3)),
					namespace:  (1 + (n+i)%2) * 1000,
					changed:    fmt.Sprintf("k%d", (n*7+batch)%5),
					createdAt:  start.Add(offset).Format("2006-01-02 15:04:05.000"),
				}
				n++
			}

			ids, err := s.writeChangeRows(ctx, rows...)
			if err != nil {
				writeErr = err
				return
			}
			finalID = ids[len(ids)-1]
		}
	}()

	var (
		upper    int64
		terms    int
		finished bool
		latest   = make(map[string]int64)
		wait     = (<-chan struct{})(writerDone)
	)
	for !finished || upper < finalID {
		select {
		case term := <-stream.Terms():
			terms++

			// The events must be above the highest id of the previous term and
			// in ascending order of id.
			seen := make(map[string]struct{})
			previous := upper
			for _, event := range termEvents(c, term) {
				c.Check(event.id > previous, tc.IsTrue, tc.Commentf("event %d is not after %d", event.id, previous))
				previous = event.id

				// The event must be all of one row.
				c.Check(event, tc.DeepEquals, s.readChangeLogRow(c, event.id))

				key := event.namespace + "/" + event.changed
				_, duplicate := seen[key]
				c.Check(duplicate, tc.IsFalse, tc.Commentf("%s seen twice in one term", key))
				seen[key] = struct{}{}
				c.Check(event.id > latest[key], tc.IsTrue, tc.Commentf("%s went back to %d from %d", key, event.id, latest[key]))
				latest[key] = event.id
			}
			if previous > upper {
				upper = previous
			}

			term.Done(false, make(chan struct{}))

		case <-wait:
			finished = true
			wait = nil
			c.Assert(writeErr, tc.ErrorIsNil)

		case <-c.Context().Done():
			c.Fatal("timed out waiting for changes")
		}
	}
	c.Logf("streamed %d rows in %d terms", finalID, terms)

	rows, err := s.DB().QueryContext(c.Context(), `
SELECT n.namespace, c.changed, MAX(c.id)
FROM change_log c JOIN change_log_namespace n ON c.namespace_id = n.id
GROUP BY n.namespace, c.changed`)
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	expected := make(map[string]int64)
	for rows.Next() {
		var (
			namespace, changed string
			id                 int64
		)
		c.Assert(rows.Scan(&namespace, &changed, &id), tc.ErrorIsNil)
		expected[namespace+"/"+changed] = id
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)
	c.Check(latest, tc.DeepEquals, expected)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestOneChangeIsBlockedByFile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAfterWithoutTermTimeout()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	notify := s.expectFileNotifyWatcher()

	s.insertNamespace(c, 1000, "foo")

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	expectNotifyBlock := func(block bool) {
		notified := make(chan bool)
		go func() {
			defer close(notified)
			notify <- block
		}()
		select {
		case <-notified:
		case <-c.Context().Done():
			c.Fatal("timed out waiting for blocking change")
		}
	}

	expectNotifyBlock(true)

	first := change{
		id:   1000,
		uuid: uuid.MustNewUUID().String(),
	}
	s.insertChange(c, first)

	select {
	case term := <-stream.Terms():
		c.Fatalf("unexpected term %+v", term)
	case <-time.After(witnessChangeLongDuration):
	}

	expectNotifyBlock(false)

	var results []changestream.ChangeEvent
	select {
	case term := <-stream.Terms():
		results = append(results, term.Changes()...)
	case <-c.Context().Done():
		c.Fatal("timed out waiting for change")
	}

	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Namespace(), tc.Equals, "foo")
	c.Check(results[0].Changed(), tc.Equals, first.uuid)

	workertest.CleanKill(c, stream)
}

func constructWatermark(start, finish int) string {
	var builder strings.Builder
	for j := start; j < finish; j++ {
		builder.WriteString(fmt.Sprintf("(lower: %d, upper: %d)", j+1, j+1))
		if j != finish-1 {
			builder.WriteString(", ")
		}
	}
	return builder.String()
}

func (s *streamSuite) TestReport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)

	s.expectTermAfterAnyTimes()
	s.expectBackoffAnyTimes(done)
	s.expectFileNotifyWatcher()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	ch := make(chan time.Time)

	s.clock.EXPECT().NewTimer(gomock.Any()).Return(s.timer).AnyTimes()
	s.timer.EXPECT().Chan().DoAndReturn(func() <-chan time.Time {
		return ch
	}).AnyTimes()
	s.timer.EXPECT().Stop()

	sync := make(chan struct{})
	s.timer.EXPECT().Reset(gomock.Any()).DoAndReturn(func(d time.Duration) bool {
		defer close(sync)
		return true
	})

	id := uuid.MustNewUUID().String()
	stream := New(id, s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	for range changestream.DefaultNumTermWatermarks {
		chg := change{
			id:   1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, chg)

		select {
		case term := <-stream.Terms():
			c.Assert(term.Changes(), tc.HasLen, 1)

			// A report during a term, shouldn't be blocked. This test proves
			// that case.
			data := stream.Report(c.Context())
			c.Check(data["last-recorded-watermark"], tc.Equals, "")

			term.Done(false, make(chan struct{}))
		case <-c.Context().Done():
			c.Fatal("timed out waiting for change")
		}
	}

	// We need to force a synchronization point, so that we actually witness
	// the change. This is because we wait until after the done channel is
	// closed before we update the watermark.
	syncPoint := func(c *tc.C) map[string]any {
		for i := range 25 {
			data := stream.Report(c.Context())
			if strings.Contains(data["watermarks"].(string), strconv.Itoa(changestream.DefaultNumTermWatermarks)) {
				return data
			}

			// Exponential backoff with a maximum of 10 seconds, we don't want
			// to wait too long, but we also want to ensure that we give the
			// worker enough time to process the change and update the
			// watermark.
			backoff := time.Duration(1<<i) * time.Millisecond
			<-time.After(min(backoff, time.Second*10))
		}
		c.Fatalf("timed out waiting for sync point")
		return nil
	}
	data := syncPoint(c)
	c.Check(data, tc.DeepEquals, map[string]any{
		"id":                      id,
		"watermarks":              constructWatermark(0, changestream.DefaultNumTermWatermarks),
		"last-recorded-watermark": "",
	})

	select {
	case ch <- time.Now().UTC():
	case <-c.Context().Done():
		c.Fatal("timed out waiting for timer")
	}

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for timer")
	}

	s.expectWaterMark(c, id, 1)

	data = stream.Report(c.Context())
	c.Check(data, tc.DeepEquals, map[string]any{
		"id":                      id,
		"watermarks":              constructWatermark(1, changestream.DefaultNumTermWatermarks),
		"last-recorded-watermark": "(lower: 1, upper: 1)",
	})

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestWatermarkWrite(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)

	s.expectTermAfterAnyTimes()
	s.expectBackoffAnyTimes(done)
	s.expectFileNotifyWatcher()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	ch := make(chan time.Time)

	s.clock.EXPECT().NewTimer(gomock.Any()).Return(s.timer).AnyTimes()
	s.timer.EXPECT().Chan().DoAndReturn(func() <-chan time.Time {
		return ch
	}).AnyTimes()
	s.timer.EXPECT().Stop()
	sync := make(chan struct{})
	s.timer.EXPECT().Reset(gomock.Any()).DoAndReturn(func(d time.Duration) bool {
		defer close(sync)
		return true
	})

	tag := uuid.MustNewUUID().String()
	stream := New(tag, s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	for i := 0; i < changestream.DefaultNumTermWatermarks; i++ {
		chg := change{
			id:   1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, chg)

		select {
		case term := <-stream.Terms():
			c.Assert(term.Changes(), tc.HasLen, 1)
			term.Done(false, make(chan struct{}))
		case <-c.Context().Done():
			c.Fatal("timed out waiting for change")
		}
	}

	select {
	case ch <- time.Now().UTC():
	case <-c.Context().Done():
		c.Fatal("timed out waiting for timer")
	}

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for timer")
	}

	s.expectWaterMark(c, tag, 1)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestWatermarkWriteIsIgnored(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)

	s.expectTermAfterAnyTimes()
	s.expectBackoffAnyTimes(done)
	s.expectFileNotifyWatcher()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	ch := make(chan time.Time)

	s.clock.EXPECT().NewTimer(gomock.Any()).Return(s.timer).AnyTimes()
	s.timer.EXPECT().Chan().DoAndReturn(func() <-chan time.Time {
		return ch
	}).AnyTimes()
	s.timer.EXPECT().Stop()
	sync := make(chan struct{})
	s.timer.EXPECT().Reset(gomock.Any()).DoAndReturn(func(d time.Duration) bool {
		defer close(sync)
		return true
	})

	tag := uuid.MustNewUUID().String()
	stream := New(tag, s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	for i := 0; i < changestream.DefaultNumTermWatermarks-1; i++ {
		chg := change{
			id:   1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, chg)

		select {
		case term := <-stream.Terms():
			c.Assert(term.Changes(), tc.HasLen, 1)
			term.Done(false, make(chan struct{}))
		case <-c.Context().Done():
			c.Fatal("timed out waiting for change")
		}
	}

	select {
	case ch <- time.Now().UTC():
	case <-c.Context().Done():
		c.Fatal("timed out waiting for timer")
	}

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for timer")
	}

	s.expectWaterMark(c, tag, -1)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestWatermarkWriteUpdatesToTheLaterOne(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)

	s.expectTermAfterAnyTimes()
	s.expectBackoffAnyTimes(done)
	s.expectFileNotifyWatcher()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	ch := make(chan time.Time)

	s.clock.EXPECT().NewTimer(gomock.Any()).Return(s.timer).AnyTimes()
	s.timer.EXPECT().Chan().DoAndReturn(func() <-chan time.Time {
		return ch
	}).AnyTimes()
	s.timer.EXPECT().Stop()
	sync := make(chan struct{})
	s.timer.EXPECT().Reset(gomock.Any()).DoAndReturn(func(d time.Duration) bool {
		defer close(sync)
		return true
	})

	tag := uuid.MustNewUUID().String()
	stream := New(tag, s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	// Insert the first change, which will be the first watermark.
	insertAndWitness := func(c *tc.C) {
		chg := change{
			id:   1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, chg)

		select {
		case term := <-stream.Terms():
			c.Assert(term.Changes(), tc.HasLen, 1)
			term.Done(false, make(chan struct{}))
		case <-c.Context().Done():
			c.Fatal("timed out waiting for change")
		}
	}

	for i := 0; i < changestream.DefaultNumTermWatermarks+2; i++ {
		insertAndWitness(c)
	}

	select {
	case ch <- time.Now().UTC():
	case <-c.Context().Done():
		c.Fatal("timed out waiting for timer")
	}

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for timer")
	}

	s.expectWaterMark(c, tag, 3)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestReadChangesWithNoChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stream := s.newNonRunningStream()

	s.insertNamespace(c, 1000, "foo")

	results, err := stream.readChanges()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.HasLen, 0)
}

func (s *streamSuite) TestReadChangesWithOneChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stream := s.newNonRunningStream()

	s.insertNamespace(c, 1000, "foo")

	first := change{
		id:   1000,
		uuid: uuid.MustNewUUID().String(),
	}
	s.insertChange(c, first)

	results, err := stream.readChanges()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Namespace(), tc.Equals, "foo")
	c.Check(results[0].Changed(), tc.Equals, first.uuid)
}

func (s *streamSuite) TestReadChangesWithMultipleSameChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stream := s.newNonRunningStream()

	s.insertNamespace(c, 1000, "foo")

	uuid := uuid.MustNewUUID().String()
	for range 10 {
		ch := change{
			id:   1000,
			uuid: uuid,
		}
		s.insertChange(c, ch)
	}

	results, err := stream.readChanges()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.HasLen, 1)
	c.Assert(results[0].Namespace(), tc.Equals, "foo")
	c.Assert(results[0].Changed(), tc.Equals, uuid)
}

func (s *streamSuite) TestReadChangesWithMultipleChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stream := s.newNonRunningStream()

	s.insertNamespace(c, 1000, "foo")

	changes := make([]change, 10)
	for i := range 10 {
		ch := change{
			id:   1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		changes[i] = ch
	}

	results, err := stream.readChanges()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.HasLen, 10)
	for i := range results {
		c.Check(results[i].Namespace(), tc.Equals, "foo")
		c.Check(results[i].Changed(), tc.Equals, changes[i].uuid)
	}
}

func (s *streamSuite) TestReadChangesWithMultipleChangesGroupsCorrectly(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stream := s.newNonRunningStream()

	s.insertNamespace(c, 1000, "foo")

	changes := make([]change, 10)
	for i := range 10 {
		var (
			ch   change
			uuid = uuid.MustNewUUID().String()
		)
		// Grouping is done via uuid, so we should only ever see the last change
		// when grouping them.
		for range 10 {
			ch = change{
				id:   1000,
				uuid: uuid,
			}
			s.insertChange(c, ch)
		}
		changes[i] = ch
	}

	results, err := stream.readChanges()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.HasLen, 10)
	for i := range results {
		c.Check(results[i].Namespace(), tc.Equals, "foo")
		c.Check(results[i].Changed(), tc.Equals, changes[i].uuid)
	}
}

func (s *streamSuite) TestReadChangesWithMultipleChangesInterweavedGroupsCorrectly(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stream := s.newNonRunningStream()

	s.insertNamespace(c, 1000, "foo")
	s.insertNamespace(c, 2000, "bar")

	// Setup for this test is a bit more complicated to ensure that interweaving
	// correctly groups the changes.

	changes := make([]change, 5)

	var (
		uuid0 = uuid.MustNewUUID().String()
		uuid1 = uuid.MustNewUUID().String()
		uuid2 = uuid.MustNewUUID().String()
	)

	{ // Group ID: 0, Row ID: 1
		ch := change{id: 1000, uuid: uuid0}
		s.insertChangeForType(c, changestreamtesting.Create, ch)
		changes[0] = ch
	}
	{ // Group ID: 1, Row ID: 2
		ch := change{id: 2000, uuid: uuid0}
		s.insertChangeForType(c, changestreamtesting.Update, ch)
		// no witness changed.
	}
	{ // Group ID: 2, Row ID: 3
		ch := change{id: 1000, uuid: uuid1}
		s.insertChangeForType(c, changestreamtesting.Update, ch)
	}
	{ // Group ID: 2, Row ID: 4
		ch := change{id: 1000, uuid: uuid1}
		s.insertChangeForType(c, changestreamtesting.Update, ch)
		// no witness changed.
	}
	{ // Group ID: 1, Row ID: 5
		ch := change{id: 2000, uuid: uuid0}
		s.insertChangeForType(c, changestreamtesting.Update, ch)
		// no witness changed.
	}
	{ // Group ID: 3, Row ID: 6
		ch := change{id: 1000, uuid: uuid2}
		s.insertChangeForType(c, changestreamtesting.Update, ch)
	}
	{ // Group ID: 3, Row ID: 7
		ch := change{id: 1000, uuid: uuid2}
		s.insertChangeForType(c, changestreamtesting.Update, ch)
		changes[1] = ch
	}
	{ // Group ID: 1, Row ID: 8
		ch := change{id: 2000, uuid: uuid0}
		s.insertChangeForType(c, changestreamtesting.Update, ch)
		changes[2] = ch
	}
	{ // Group ID: 2, Row ID: 9
		ch := change{id: 1000, uuid: uuid1}
		// In theory this should never happen because we're using transactions,
		// so we should always witness a creation before an update. However,
		// this part of the tests states that we will still witness the
		// creation  after an update if something goes wrong.
		s.insertChangeForType(c, changestreamtesting.Create, ch)
		changes[3] = ch
	}

	results, err := stream.readChanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 4, tc.Commentf("expected 4, received %v", len(results)))

	type changeResults struct {
		changeType changestream.ChangeType
		namespace  string
		uuid       string
	}

	expected := []changeResults{
		{changeType: changestreamtesting.Create, namespace: "foo", uuid: uuid0},
		{changeType: changestreamtesting.Update, namespace: "foo", uuid: uuid2},
		{changeType: changestreamtesting.Update, namespace: "bar", uuid: uuid0},
		{changeType: changestreamtesting.Create, namespace: "foo", uuid: uuid1},
	}

	c.Logf("result %v", results)
	for i := range results {
		c.Logf("expected %v", expected[i])
		c.Check(results[i].Type(), tc.Equals, expected[i].changeType)
		c.Check(results[i].Namespace(), tc.Equals, expected[i].namespace)
		c.Check(results[i].Changed(), tc.Equals, expected[i].uuid)
	}
}

// expectedEvent describes an event that readChanges is expected to return for
// the rows inserted by insertLatestRowFixture.
type expectedEvent struct {
	// row is the index, in insertion order, of the fixture row that the event
	// must be built from.
	row        int
	changeType changestream.ChangeType
	namespace  string
	changed    string
	createdAt  string
}

// TestReadChangesReturnsLatestRowOfEachChange checks that when several changes
// to the same namespace and changed value are coalesced, the event is built
// entirely from the row with the highest id: its id, edit type, namespace,
// changed value and created_at all come from that one row.
//
// The fixture makes the latest row of foo/k1 differ from every other row of
// the group in each column: it is not the first row, its edit type is neither
// the highest nor the lowest and its created_at is neither the latest nor the
// earliest. Building the event from any other row of the group (the result a
// bare, non-aggregated column is allowed to take in standard SQL), or from a
// per column MIN or MAX, would therefore produce a different event.
// The same changed value is used in all three namespaces, which must remain
// three independent events, and the events must be ordered by id, which here
// matches neither the namespace, the changed value nor created_at order.
func (s *streamSuite) TestReadChangesReturnsLatestRowOfEachChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stream := s.newNonRunningStream()
	ids := s.insertLatestRowFixture(c)

	results, err := stream.readChanges()
	c.Assert(err, tc.ErrorIsNil)

	s.expectEvents(c, ids, results, []expectedEvent{
		{row: 4, changeType: changestreamtesting.Update, namespace: "bar", changed: "k1", createdAt: "2026-01-02 10:00:06.000"},
		{row: 5, changeType: changestreamtesting.Create, namespace: "baz", changed: "k1", createdAt: "2026-01-02 10:00:07.000"},
		{row: 6, changeType: changestreamtesting.Update, namespace: "foo", changed: "k1", createdAt: "2026-01-02 10:00:04.000"},
		{row: 7, changeType: changestreamtesting.Delete, namespace: "foo", changed: "k2", createdAt: "2026-01-02 10:00:08.000"},
	})
}

// TestReadChangesOnlyReturnsRowsAboveUpperBound checks that rows at or below
// the upper bound of the last processed term are never returned, that the
// bound is exclusive and that a namespace and changed value whose latest row is
// not above the bound produces no event at all.
func (s *streamSuite) TestReadChangesOnlyReturnsRowsAboveUpperBound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ids := s.insertLatestRowFixture(c)

	// The events for the fixture, by the index of the row they are built
	// from, ordered by id.
	events := map[int]expectedEvent{
		4: {row: 4, changeType: changestreamtesting.Update, namespace: "bar", changed: "k1", createdAt: "2026-01-02 10:00:06.000"},
		5: {row: 5, changeType: changestreamtesting.Create, namespace: "baz", changed: "k1", createdAt: "2026-01-02 10:00:07.000"},
		6: {row: 6, changeType: changestreamtesting.Update, namespace: "foo", changed: "k1", createdAt: "2026-01-02 10:00:04.000"},
		7: {row: 7, changeType: changestreamtesting.Delete, namespace: "foo", changed: "k2", createdAt: "2026-01-02 10:00:08.000"},
	}

	tests := []struct {
		name string
		// upper is the index of the row whose id is the upper bound, or -1
		// for no bound.
		upper int
		rows  []int
	}{
		{name: "no bound", upper: -1, rows: []int{4, 5, 6, 7}},
		// Rows 0 to 3 are ignored. Every key still has a later row, so the
		// events are unchanged.
		{name: "bound below every latest row", upper: 3, rows: []int{4, 5, 6, 7}},
		// bar/k1 has its latest row at the bound, so it is excluded.
		{name: "bound at a latest row", upper: 4, rows: []int{5, 6, 7}},
		{name: "bound between latest rows", upper: 5, rows: []int{6, 7}},
		{name: "bound at the penultimate row", upper: 6, rows: []int{7}},
		{name: "bound at the last row", upper: 7, rows: nil},
	}
	for _, test := range tests {
		c.Logf("test: %s", test.name)

		stream := s.newNonRunningStream()
		if test.upper >= 0 {
			stream.recordTermView(&termView{lower: ids[0], upper: ids[test.upper]})
		}

		results, err := stream.readChanges()
		c.Assert(err, tc.ErrorIsNil)

		var expected []expectedEvent
		for _, row := range test.rows {
			expected = append(expected, events[row])
		}
		s.expectEvents(c, ids, results, expected)
	}
}

// TestReadChangesIgnoresPrunedRows checks that pruning the change log, which
// removes rows up to the lower bound of the lowest recorded term, does not
// change the events returned for the rows above the upper bound. The pruner
// never removes a row above any term's lower bound, so events are always built
// from rows that are still present.
func (s *streamSuite) TestReadChangesIgnoresPrunedRows(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ids := s.insertLatestRowFixture(c)

	stream := s.newNonRunningStream()
	stream.recordTermView(&termView{lower: ids[2], upper: ids[4]})

	expected := []expectedEvent{
		{row: 5, changeType: changestreamtesting.Create, namespace: "baz", changed: "k1", createdAt: "2026-01-02 10:00:07.000"},
		{row: 6, changeType: changestreamtesting.Update, namespace: "foo", changed: "k1", createdAt: "2026-01-02 10:00:04.000"},
		{row: 7, changeType: changestreamtesting.Delete, namespace: "foo", changed: "k2", createdAt: "2026-01-02 10:00:08.000"},
	}

	// Prune progressively further, up to the lower bound of the recorded term,
	// as the change log pruner does.
	for _, pruneTo := range []int{0, 1, 2} {
		_, err := s.DB().ExecContext(c.Context(), `DELETE FROM change_log WHERE id <= ?`, ids[pruneTo])
		c.Assert(err, tc.ErrorIsNil)

		results, err := stream.readChanges()
		c.Assert(err, tc.ErrorIsNil)

		s.expectEvents(c, ids, results, expected)
	}
}

func (s *streamSuite) TestProcessWatermark(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectMetrics()

	stream := s.newNonRunningStream()

	err := stream.processWatermark(func(tv *termView) error {
		c.Fatalf("unexpected call to process watermark")
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Insert 1 item into the buffer. This will be the first watermark. As the
	// buffer isn't full we should not see a process watermark call.
	stream.recordTermView(&termView{lower: 1, upper: 2})

	err = stream.processWatermark(func(tv *termView) error {
		c.Fatalf("unexpected call to process watermark")
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Fill the buffer and witness the view.
	for i := int64(0); i < int64(changestream.DefaultNumTermWatermarks-1); i++ {
		stream.recordTermView(&termView{lower: i + 2, upper: i + 3})
	}

	witnessWatermark := func(lower, upper int64) {
		// Ensure that we witness the watermark.
		var called bool
		err = stream.processWatermark(func(tv *termView) error {
			called = true
			c.Check(tv.lower, tc.Equals, lower)
			c.Check(tv.upper, tc.Equals, upper)
			return nil
		})
		c.Check(err, tc.ErrorIsNil)
		c.Check(called, tc.IsTrue)

		// We won't witness the watermark again until we've added another term view.
		err = stream.processWatermark(func(tv *termView) error {
			c.Fatalf("unexpected call to process watermark")
			return nil
		})
		c.Check(err, tc.ErrorIsNil)
	}

	witnessWatermark(1, 2)

	// Adding a term view should trigger the watermark again.
	expected := int64(2)
	for i := changestream.DefaultNumTermWatermarks; i < changestream.DefaultNumTermWatermarks+20; i++ {
		stream.recordTermView(&termView{lower: int64(i + 1), upper: int64(i + 2)})

		witnessWatermark(expected, expected+1)
		expected++
	}
}

func (s *streamSuite) TestProcessWatermarkBufferFull(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectMetrics()

	stream := s.newNonRunningStream()

	err := stream.processWatermark(func(tv *termView) error {
		c.Fatalf("unexpected call to process watermark")
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Overfilling the buffer should cause us to witness the watermark. The
	// buffer is capped FIFO, so we will only witness the last view of the
	// buffer.
	total := int64(changestream.DefaultNumTermWatermarks * 10)
	for i := range total {
		stream.recordTermView(&termView{lower: i, upper: i + 1})
	}

	witnessWatermark := func(lower, upper int64) {
		// Ensure that we witness the watermark.
		var called bool
		err = stream.processWatermark(func(tv *termView) error {
			called = true
			c.Check(tv.lower, tc.Equals, lower)
			c.Check(tv.upper, tc.Equals, upper)
			return nil
		})
		c.Check(err, tc.ErrorIsNil)
		c.Check(called, tc.IsTrue)

		// We won't witness the watermark again until we've added another term view.
		err = stream.processWatermark(func(tv *termView) error {
			c.Fatalf("unexpected call to process watermark")
			return nil
		})
		c.Check(err, tc.ErrorIsNil)
	}

	witnessWatermark(total-int64(changestream.DefaultNumTermWatermarks), total-int64(changestream.DefaultNumTermWatermarks-1))
}

func (s *streamSuite) TestUpperBound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectMetrics()

	stream := s.newNonRunningStream()

	c.Check(stream.upperBound(), tc.Equals, int64(-1))

	// Fill the buffer and witness the view.
	for i := int64(0); i < int64(changestream.DefaultNumTermWatermarks); i++ {
		stream.recordTermView(&termView{lower: i + 2, upper: i + 3})

		c.Check(stream.upperBound(), tc.Equals, i+3)
	}

	for i := 0; i < changestream.DefaultNumTermWatermarks; i++ {
		err := stream.processWatermark(func(tv *termView) error {
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)

		c.Check(stream.upperBound(), tc.Equals, int64(changestream.DefaultNumTermWatermarks+2))
	}

	err := stream.processWatermark(func(tv *termView) error {
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(stream.upperBound(), tc.Equals, int64(changestream.DefaultNumTermWatermarks+2))
}

func (s *streamSuite) TestCreateWatermarkTwice(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stream := s.newNonRunningStream()
	err := stream.createWatermark()
	c.Assert(err, tc.ErrorIsNil)

	err = stream.createWatermark()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *streamSuite) newNonRunningStream() *Stream {
	return &Stream{
		db:             s.TxnRunner(),
		id:             uuid.MustNewUUID().String(),
		metrics:        s.metrics,
		watermarks:     make([]*termView, changestream.DefaultNumTermWatermarks),
		internalStates: nil,
	}
}

func (s *streamSuite) insertNamespace(c *tc.C, id int, name string) {
	q := `
INSERT INTO change_log_namespace VALUES (?, ?, ?);
`[1:]
	_, err := s.DB().Exec(q, id, name, "blah")
	c.Assert(err, tc.ErrorIsNil)
}

type change struct {
	id   int
	uuid string
}

func (s *streamSuite) insertChange(c *tc.C, changes ...change) {
	s.insertChangeForType(c, 2, changes...)
}

func (s *streamSuite) insertChangeForType(c *tc.C, changeType changestream.ChangeType, changes ...change) {
	q := `INSERT INTO change_log (edit_type_id, namespace_id, changed) VALUES (?, ?, ?)`
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, v := range changes {
			c.Logf("Executing insert change: edit-type: %d, %v %v", changeType, v.id, v.uuid)
			if _, err := tx.ExecContext(ctx, q, changeType, v.id, v.uuid); err != nil {
				return err
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Logf("Committed insert change")
}

// changeRow describes a change_log row to insert with an explicit created_at.
type changeRow struct {
	changeType changestream.ChangeType
	namespace  int
	changed    string
	createdAt  string
}

// insertChangeRows inserts the rows in a single transaction and returns the
// ids assigned to them, in order.
func (s *streamSuite) insertChangeRows(c *tc.C, rows ...changeRow) []int64 {
	ids, err := s.writeChangeRows(c.Context(), rows...)
	c.Assert(err, tc.ErrorIsNil)
	return ids
}

// writeChangeRows is insertChangeRows without assertions, so that it can be
// used from goroutines other than the test's.
func (s *streamSuite) writeChangeRows(ctx context.Context, rows ...changeRow) ([]int64, error) {
	q := `INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at) VALUES (?, ?, ?, ?)`

	var ids []int64
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		ids = ids[:0]
		for _, row := range rows {
			result, err := tx.ExecContext(ctx, q, row.changeType, row.namespace, row.changed, row.createdAt)
			if err != nil {
				return err
			}
			id, err := result.LastInsertId()
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return nil
	})
	return ids, err
}

// insertLatestRowFixture inserts the namespaces foo, bar and baz and eight
// change_log rows, and returns their ids in insertion order. Rows are
// interleaved across namespaces and changed values, and the same changed value
// is used in all three namespaces. created_at values are distinct and are not
// in id order.
//
//	row  namespace/changed  edit type  created_at
//	 0   foo/k1             create     10:00:01
//	 1   bar/k1             create     10:00:02
//	 2   foo/k2             update     10:00:03
//	 3   foo/k1             delete     10:00:05
//	 4   bar/k1             update     10:00:06
//	 5   baz/k1             create     10:00:07
//	 6   foo/k1             update     10:00:04
//	 7   foo/k2             delete     10:00:08
//
// The latest rows are therefore 4 (bar/k1), 5 (baz/k1), 6 (foo/k1) and
// 7 (foo/k2).
func (s *streamSuite) insertLatestRowFixture(c *tc.C) []int64 {
	s.insertNamespace(c, 1000, "foo")
	s.insertNamespace(c, 2000, "bar")
	s.insertNamespace(c, 3000, "baz")

	return s.insertChangeRows(c,
		changeRow{changeType: changestreamtesting.Create, namespace: 1000, changed: "k1", createdAt: "2026-01-02 10:00:01.000"},
		changeRow{changeType: changestreamtesting.Create, namespace: 2000, changed: "k1", createdAt: "2026-01-02 10:00:02.000"},
		changeRow{changeType: changestreamtesting.Update, namespace: 1000, changed: "k2", createdAt: "2026-01-02 10:00:03.000"},
		changeRow{changeType: changestreamtesting.Delete, namespace: 1000, changed: "k1", createdAt: "2026-01-02 10:00:05.000"},
		changeRow{changeType: changestreamtesting.Update, namespace: 2000, changed: "k1", createdAt: "2026-01-02 10:00:06.000"},
		changeRow{changeType: changestreamtesting.Create, namespace: 3000, changed: "k1", createdAt: "2026-01-02 10:00:07.000"},
		changeRow{changeType: changestreamtesting.Update, namespace: 1000, changed: "k1", createdAt: "2026-01-02 10:00:04.000"},
		changeRow{changeType: changestreamtesting.Delete, namespace: 1000, changed: "k2", createdAt: "2026-01-02 10:00:08.000"},
	)
}

// readChangeLogRow returns the change_log row with the given id as an event.
func (s *streamSuite) readChangeLogRow(c *tc.C, id int64) changeEvent {
	q := `
SELECT c.id, c.edit_type_id, n.namespace, c.changed, c.created_at
FROM change_log c JOIN change_log_namespace n ON c.namespace_id = n.id
WHERE c.id = ?`

	var row changeEvent
	err := s.DB().QueryRowContext(c.Context(), q, id).Scan(
		&row.id, &row.changeType, &row.namespace, &row.changed, &row.createdAt,
	)
	c.Assert(err, tc.ErrorIsNil)
	return row
}

// termEvents returns the changes of a term as the events built by the stream.
func termEvents(c *tc.C, term changestream.Term) []changeEvent {
	var events []changeEvent
	for _, ch := range term.Changes() {
		event, ok := ch.(changeEvent)
		c.Assert(ok, tc.IsTrue, tc.Commentf("unexpected change event type %T", ch))
		events = append(events, event)
	}
	return events
}

// expectEvents checks that the events are exactly the expected ones, in order,
// comparing every field of the event with the row that it must be built from.
// ids are the ids of the rows, in insertion order. The expected created_at is
// the value stored in the row. The driver returns a DATETIME column as a time,
// which the event holds formatted as RFC 3339.
func (s *streamSuite) expectEvents(c *tc.C, ids []int64, obtained []changeEvent, expected []expectedEvent) {
	c.Assert(obtained, tc.HasLen, len(expected))
	for i, event := range expected {
		createdAt, err := time.Parse("2006-01-02 15:04:05.000", event.createdAt)
		c.Assert(err, tc.ErrorIsNil)

		c.Check(obtained[i], tc.DeepEquals, changeEvent{
			id:         ids[event.row],
			changeType: int(event.changeType),
			namespace:  event.namespace,
			changed:    event.changed,
			createdAt:  createdAt.Format(time.RFC3339Nano),
		})
	}
}

func expectChanges(c *tc.C, expected []change, obtained []changestream.ChangeEvent) {
	c.Assert(obtained, tc.HasLen, len(expected))

	for i, chg := range expected {
		c.Check(obtained[i].Namespace(), tc.Equals, "foo")
		c.Check(obtained[i].Changed(), tc.Equals, chg.uuid)
	}
}

func (s *streamSuite) expectWaterMark(c *tc.C, id string, changeLogIndex int) {
	row := s.DB().QueryRowContext(c.Context(), "SELECT controller_id, lower_bound, upper_bound, updated_at FROM change_log_witness")

	type witness struct {
		id                     string
		lowerBound, upperBound int
		updatedAt              time.Time
	}
	var w witness
	err := row.Scan(&w.id, &w.lowerBound, &w.upperBound, &w.updatedAt)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(w.id, tc.Equals, id)
	c.Check(w.lowerBound, tc.Equals, changeLogIndex)
	c.Check(w.upperBound >= changeLogIndex, tc.IsTrue)
	c.Check(w.updatedAt, tc.Not(tc.Equals), time.Time{})
}
