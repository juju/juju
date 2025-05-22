// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stream

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/changestream"
	changestreamtesting "github.com/juju/juju/core/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
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
	defer goleak.VerifyNone(t)
	tc.Run(t, &streamSuite{})
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
	case <-time.After(testing.LongWait):
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
	case <-time.After(testing.LongWait):
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

	case <-time.After(testing.LongWait):
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

	case <-time.After(testing.LongWait):
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

	case <-time.After(testing.LongWait):
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

	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for change")
	}

	expectChanges(c, []change{chg}, results)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestOneChangeWithClosedAbort(c *tc.C) {
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

	var results []changestream.ChangeEvent
	select {
	case term := <-stream.Terms():
		results = term.Changes()

		// Force the close of the abort channel.
		ch := make(chan struct{})
		close(ch)
		term.Done(false, ch)

	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for change")
	}

	expectChanges(c, []change{chg}, results)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestOneChangeWithDelayedTermDone(c *tc.C) {
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

	case <-time.After(testing.LongWait):
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

	case <-time.After(testing.LongWait):
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
		ch <- time.Now()
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

	case <-time.After(testing.LongWait):
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

	for i := 0; i < 10; i++ {
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

		case <-time.After(testing.LongWait):
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
			case ch <- time.Now():
			case <-done:
			}
		}()
		return ch
	}).AnyTimes()

	s.insertNamespace(c, 1000, "foo")

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

	for i := 0; i < 10; i++ {
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

		case <-time.After(testing.LongWait):
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
	s.expectTermAfterAnyTimes()
	s.expectAnyAfterAnyTimes()
	s.expectTimer()
	s.expectClock()
	s.expectMetrics()

	s.insertNamespace(c, 1000, "foo")

	stream := New(uuid.MustNewUUID().String(), s.TxnRunner(), s.FileNotifier, s.clock, s.metrics, loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, stream)

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

	case <-time.After(testing.LongWait):
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

	case <-time.After(testing.LongWait):
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
	for i := 0; i < 4; i++ {
		ch := change{
			id:   1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	// Force a coalesce change through, we should not see three changes, instead
	// we should just see one.
	for i := 0; i < 2; i++ {
		s.insertChange(c, inserts[len(inserts)-1])
	}

	for i := 0; i < 4; i++ {
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
	case <-time.After(testing.LongWait):
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
	for i := 0; i < 10; i++ {
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
	case <-time.After(testing.LongWait):
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
	for i := 0; i < 4; i++ {
		ch := change{
			id:   ((i % 2) + 1) * 1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	// Force a coalesce change through, we should not see three changes, instead
	// we should just see one.
	for i := 0; i < 2; i++ {
		s.insertChange(c, inserts[len(inserts)-1])
	}

	for i := 0; i < 4; i++ {
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
	case <-time.After(testing.LongWait):
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
	for i := 0; i < 4; i++ {
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

	for i := 0; i < 4; i++ {
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
	case <-time.After(testing.LongWait):
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

func (s *streamSuite) TestOneChangeIsBlockedByFile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectTermAfterAnyTimes()
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
		case <-time.After(testing.LongWait):
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
	case <-time.After(testing.LongWait):
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

	for i := 0; i < changestream.DefaultNumTermWatermarks; i++ {
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
			data := stream.Report()
			c.Check(data["last-recorded-watermark"], tc.Equals, "")

			term.Done(false, make(chan struct{}))
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for change")
		}
	}

	// We need to force a synchronization point, so that we actually witness
	// the change. This is because we wait until after the done channel is
	// closed before we update the watermark.
	syncPoint := func(c *tc.C) map[string]any {
		for i := 0; i < 3; i++ {
			data := stream.Report()
			if strings.Contains(data["watermarks"].(string), strconv.Itoa(changestream.DefaultNumTermWatermarks)) {
				return data
			}
			<-time.After(testing.LongWait)
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
	case ch <- time.Now():
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for timer")
	}

	select {
	case <-sync:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for timer")
	}

	s.expectWaterMark(c, id, 1)

	data = stream.Report()
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
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for change")
		}
	}

	select {
	case ch <- time.Now():
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for timer")
	}

	select {
	case <-sync:
	case <-time.After(testing.LongWait):
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
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for change")
		}
	}

	select {
	case ch <- time.Now():
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for timer")
	}

	select {
	case <-sync:
	case <-time.After(testing.LongWait):
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
	insertAndWitness := func(c *tc.C, id int) {
		chg := change{
			id:   1000,
			uuid: uuid.MustNewUUID().String(),
		}
		s.insertChange(c, chg)

		select {
		case term := <-stream.Terms():
			c.Assert(term.Changes(), tc.HasLen, 1)
			term.Done(false, make(chan struct{}))
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for change")
		}
	}

	for i := 0; i < changestream.DefaultNumTermWatermarks+2; i++ {
		insertAndWitness(c, i+1)
	}

	select {
	case ch <- time.Now():
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for timer")
	}

	select {
	case <-sync:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for timer")
	}

	s.expectWaterMark(c, tag, 3)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestReadChangesWithNoChanges(c *tc.C) {
	stream := s.newStream()

	s.insertNamespace(c, 1000, "foo")

	results, err := stream.readChanges()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.HasLen, 0)
}

func (s *streamSuite) TestReadChangesWithOneChange(c *tc.C) {
	stream := s.newStream()

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
	stream := s.newStream()

	s.insertNamespace(c, 1000, "foo")

	uuid := uuid.MustNewUUID().String()
	for i := 0; i < 10; i++ {
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
	stream := s.newStream()

	s.insertNamespace(c, 1000, "foo")

	changes := make([]change, 10)
	for i := 0; i < 10; i++ {
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
	stream := s.newStream()

	s.insertNamespace(c, 1000, "foo")

	changes := make([]change, 10)
	for i := 0; i < 10; i++ {
		var (
			ch   change
			uuid = uuid.MustNewUUID().String()
		)
		// Grouping is done via uuid, so we should only ever see the last change
		// when grouping them.
		for j := 0; j < 10; j++ {
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
	stream := s.newStream()

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

func (s *streamSuite) TestProcessWatermark(c *tc.C) {
	stream := s.newStream()

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
	stream := s.newStream()

	err := stream.processWatermark(func(tv *termView) error {
		c.Fatalf("unexpected call to process watermark")
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Overfilling the buffer should cause us to witness the watermark. The
	// buffer is capped FIFO, so we will only witness the last view of the
	// buffer.
	total := int64(changestream.DefaultNumTermWatermarks * 10)
	for i := int64(0); i < total; i++ {
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
	stream := s.newStream()

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
	stream := s.newStream()
	err := stream.createWatermark()
	c.Assert(err, tc.ErrorIsNil)

	err = stream.createWatermark()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *streamSuite) newStream() *Stream {
	return &Stream{
		db:         s.TxnRunner(),
		id:         uuid.MustNewUUID().String(),
		metrics:    s.metrics,
		watermarks: make([]*termView, changestream.DefaultNumTermWatermarks),
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
