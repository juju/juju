// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	time "time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/testing"
)

type streamSuite struct {
	baseSuite
}

var _ = gc.Suite(&streamSuite{})

func (s *streamSuite) TestLoopWithNoTicks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectFileNotifyWatcher()
	s.expectTimer(0)

	stream, err := NewStream(s.TrackedDB(), s.fileNotifier, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, stream)

	changes := stream.Changes()
	c.Assert(changes, gc.HasLen, 0)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestNoData(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectFileNotifyWatcher()
	done := s.expectTimer(1)

	stream, err := NewStream(s.TrackedDB(), s.fileNotifier, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, stream)

	changes := stream.Changes()
	c.Assert(changes, gc.HasLen, 0)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for timer to fire")
	}

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestOneChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectFileNotifyWatcher()
	done := s.expectTimer(1)

	s.insertNamespace(c, 1, "foo")

	first := change{
		id:   1,
		uuid: utils.MustNewUUID().String(),
	}
	s.insertChange(c, first)

	stream, err := NewStream(s.TrackedDB(), s.fileNotifier, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, stream)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for timer to fire")
	}

	changes := stream.Changes()

	var results []changestream.ChangeEvent
	select {
	case change := <-changes:
		results = append(results, change)
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for change")
	}

	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Namespace(), gc.Equals, "foo")
	c.Assert(results[0].ChangedUUID(), gc.Equals, first.uuid)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestMultipleChanges(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectFileNotifyWatcher()
	done := s.expectTimer(1)

	s.insertNamespace(c, 1, "foo")

	var inserts []change
	for i := 0; i < 10; i++ {
		ch := change{
			id:   1,
			uuid: utils.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream, err := NewStream(s.TrackedDB(), s.fileNotifier, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, stream)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for timer to fire")
	}

	changes := stream.Changes()

	var results []changestream.ChangeEvent
	for i := 0; i < 10; i++ {
		select {
		case change := <-changes:
			results = append(results, change)
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting for change")
		}
	}

	c.Assert(results, gc.HasLen, 10)
	for i, result := range results {
		idx := len(results) - 1 - i
		c.Assert(result.Namespace(), gc.Equals, "foo")
		c.Assert(result.ChangedUUID(), gc.Equals, inserts[idx].uuid)
	}

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestMultipleChangesWithSameUUIDCoalesce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectFileNotifyWatcher()
	done := s.expectTimer(1)

	s.insertNamespace(c, 1, "foo")

	var inserts []change
	for i := 0; i < 4; i++ {
		ch := change{
			id:   1,
			uuid: utils.MustNewUUID().String(),
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
			id:   1,
			uuid: utils.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream, err := NewStream(s.TrackedDB(), s.fileNotifier, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, stream)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for timer to fire")
	}

	changes := stream.Changes()

	var results []changestream.ChangeEvent
	for i := 0; i < 8; i++ {
		select {
		case change := <-changes:
			results = append(results, change)
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting for change")
		}
	}

	c.Assert(results, gc.HasLen, 8)
	for i, result := range results {
		idx := len(results) - 1 - i
		c.Assert(result.Namespace(), gc.Equals, "foo")
		c.Assert(result.ChangedUUID(), gc.Equals, inserts[idx].uuid)
	}

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestMultipleChangesWithNamespaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectFileNotifyWatcher()
	done := s.expectTimer(1)

	s.insertNamespace(c, 1, "foo")
	s.insertNamespace(c, 2, "bar")

	var inserts []change
	for i := 0; i < 10; i++ {
		ch := change{
			id:   (i % 2) + 1,
			uuid: utils.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream, err := NewStream(s.TrackedDB(), s.fileNotifier, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, stream)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for timer to fire")
	}

	changes := stream.Changes()

	var results []changestream.ChangeEvent
	for i := 0; i < 10; i++ {
		select {
		case change := <-changes:
			results = append(results, change)
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting for change")
		}
	}

	c.Assert(results, gc.HasLen, 10)
	for i, result := range results {
		idx := len(results) - 1 - i
		namespace := "foo"
		if inserts[idx].id == 2 {
			namespace = "bar"
		}
		c.Assert(result.Namespace(), gc.Equals, namespace)
		c.Assert(result.ChangedUUID(), gc.Equals, inserts[idx].uuid)
	}

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestMultipleChangesWithNamespacesCoalesce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectFileNotifyWatcher()
	done := s.expectTimer(1)

	s.insertNamespace(c, 1, "foo")
	s.insertNamespace(c, 2, "bar")

	var inserts []change
	for i := 0; i < 4; i++ {
		ch := change{
			id:   (i % 2) + 1,
			uuid: utils.MustNewUUID().String(),
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
			id:   (i % 2) + 1,
			uuid: utils.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream, err := NewStream(s.TrackedDB(), s.fileNotifier, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, stream)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for timer to fire")
	}

	changes := stream.Changes()

	var results []changestream.ChangeEvent
	for i := 0; i < 8; i++ {
		select {
		case change := <-changes:
			results = append(results, change)
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting for change")
		}
	}

	c.Assert(results, gc.HasLen, 8)
	for i, result := range results {
		idx := len(results) - 1 - i
		namespace := "foo"
		if inserts[idx].id == 2 {
			namespace = "bar"
		}
		c.Assert(result.Namespace(), gc.Equals, namespace)
		c.Assert(result.ChangedUUID(), gc.Equals, inserts[idx].uuid)
	}

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestMultipleChangesWithNoNamespacesDoNotCoalesce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectFileNotifyWatcher()
	done := s.expectTimer(1)

	s.insertNamespace(c, 1, "foo")
	s.insertNamespace(c, 2, "bar")
	s.insertNamespace(c, 3, "baz")

	var inserts []change
	for i := 0; i < 4; i++ {
		ch := change{
			id:   (i % 2) + 1,
			uuid: utils.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	// Force a non coalesced change through. It has the same UUID, but not
	// the same namespace, so should come through as another change.
	ch := change{
		id:   3,
		uuid: inserts[len(inserts)-1].uuid,
	}
	s.insertChange(c, ch)
	inserts = append(inserts, ch)

	// Force a coalesced change through. It has the same UUID and namespace,
	// so we should only see one change.
	s.insertChange(c, inserts[len(inserts)-1])

	for i := 0; i < 4; i++ {
		ch := change{
			id:   (i % 2) + 1,
			uuid: utils.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream, err := NewStream(s.TrackedDB(), s.fileNotifier, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, stream)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for timer to fire")
	}

	changes := stream.Changes()

	var results []changestream.ChangeEvent
	for i := 0; i < 9; i++ {
		select {
		case change := <-changes:
			results = append(results, change)
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting for change")
		}
	}

	c.Assert(results, gc.HasLen, 9)
	for i, result := range results {
		idx := len(results) - 1 - i
		namespace := "foo"
		if inserts[idx].id == 2 {
			namespace = "bar"
		} else if inserts[idx].id == 3 {
			namespace = "baz"
		}
		c.Assert(result.Namespace(), gc.Equals, namespace)
		c.Assert(result.ChangedUUID(), gc.Equals, inserts[idx].uuid)
	}

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestOneChangeIsBlockedByFile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	notify := s.expectFileNotifyWatcher()

	s.insertNamespace(c, 1, "foo")

	stream, err := NewStream(s.TrackedDB(), s.fileNotifier, s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, stream)

	timeTick := s.setupTimer()
	changes := stream.Changes()

	assertChange := func(expected func(<-chan changestream.ChangeEvent, string) (string, bool)) string {
		done := s.expectTick(timeTick, 1)

		change := change{
			id:   1,
			uuid: utils.MustNewUUID().String(),
		}
		s.insertChange(c, change)

		uuid, witnessTick := expected(changes, change.uuid)
		if !witnessTick {
			return uuid
		}

		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for timer to fire")
		}

		return uuid
	}
	expectOneChange := func(changes <-chan changestream.ChangeEvent, uuid string) (string, bool) {
		var results []changestream.ChangeEvent
		select {
		case change := <-changes:
			results = append(results, change)
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for change")
		}

		c.Assert(results, gc.HasLen, 1)
		c.Assert(results[0].Namespace(), gc.Equals, "foo")
		c.Assert(results[0].ChangedUUID(), gc.Equals, uuid)

		return uuid, true
	}
	expectNoChange := func(changes <-chan changestream.ChangeEvent, uuid string) (string, bool) {
		select {
		case <-changes:
			c.Fatal("timed out waiting for change")
		case <-time.After(time.Second):
		}
		return uuid, false
	}
	expectNotify := func(block bool) {
		notified := make(chan bool)
		go func() {
			defer close(notified)
			notify <- block
		}()
		select {
		case <-notified:
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for change")
		}
	}

	assertChange(expectOneChange)

	expectNotify(true)

	uuid := assertChange(expectNoChange)

	expectNotify(false)

	expectOneChange(changes, uuid)

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) insertNamespace(c *gc.C, id int, name string) {
	q := `
INSERT INTO change_log_namespace VALUES (?, ?);
`[1:]
	_, err := s.DB().Exec(q, id, name)
	c.Assert(err, jc.ErrorIsNil)
}

type change struct {
	id   int
	uuid string
}

func (s *streamSuite) insertChange(c *gc.C, changes ...change) {
	q := `
INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid)
VALUES (2, ?, ?)
`[1:]

	tx, err := s.DB().Begin()
	c.Assert(err, jc.ErrorIsNil)

	stmt, err := tx.Prepare(q)
	c.Assert(err, jc.ErrorIsNil)

	for _, v := range changes {
		c.Logf("Executing insert change: %v %v", v.id, v.uuid)
		_, err = stmt.Exec(v.id, v.uuid)
		c.Assert(err, jc.ErrorIsNil)
	}

	c.Logf("Committing insert change")
	err = tx.Commit()
	c.Assert(err, jc.ErrorIsNil)
}
