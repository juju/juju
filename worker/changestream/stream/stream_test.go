// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stream

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/testing"
)

const (
	// We need to ensure that we never witness a change event. We've picked
	// an arbitrary timeout of 1 second, which should be more than enough
	// time for the worker to process the change.
	witnessChangeDuration = time.Second
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

	stream := New(s.TrackedDB(), s.FileNotifier, s.clock, s.logger)
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

	stream := New(s.TrackedDB(), s.FileNotifier, s.clock, s.logger)
	defer workertest.DirtyKill(c, stream)

	changes := stream.Changes()

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for timer to fire")
	}

	// Synchronization block to ensure that we don't witness any changes.
	select {
	case <-time.After(witnessChangeDuration):
		c.Assert(changes, gc.HasLen, 0)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for changes")
	}

	workertest.CleanKill(c, stream)
}

func (s *streamSuite) TestOneChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectFileNotifyWatcher()
	done := s.expectTimer(1)

	s.insertNamespace(c, 1000, "foo")

	first := change{
		id:   1000,
		uuid: utils.MustNewUUID().String(),
	}
	s.insertChange(c, first)

	stream := New(s.TrackedDB(), s.FileNotifier, s.clock, s.logger)
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

	s.insertNamespace(c, 1000, "foo")

	var inserts []change
	for i := 0; i < 10; i++ {
		ch := change{
			id:   1000,
			uuid: utils.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream := New(s.TrackedDB(), s.FileNotifier, s.clock, s.logger)
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

	s.insertNamespace(c, 1000, "foo")

	var inserts []change
	for i := 0; i < 4; i++ {
		ch := change{
			id:   1000,
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
			id:   1000,
			uuid: utils.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream := New(s.TrackedDB(), s.FileNotifier, s.clock, s.logger)
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

	s.insertNamespace(c, 1000, "foo")
	s.insertNamespace(c, 2000, "bar")

	var inserts []change
	for i := 0; i < 10; i++ {
		ch := change{
			id:   ((i % 2) + 1) * 1000,
			uuid: utils.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream := New(s.TrackedDB(), s.FileNotifier, s.clock, s.logger)
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
		if inserts[idx].id == 2000 {
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

	s.insertNamespace(c, 1000, "foo")
	s.insertNamespace(c, 2000, "bar")

	var inserts []change
	for i := 0; i < 4; i++ {
		ch := change{
			id:   ((i % 2) + 1) * 1000,
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
			id:   ((i % 2) + 1) * 1000,
			uuid: utils.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream := New(s.TrackedDB(), s.FileNotifier, s.clock, s.logger)
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
		if inserts[idx].id == 2000 {
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

	s.insertNamespace(c, 1000, "foo")
	s.insertNamespace(c, 2000, "bar")
	s.insertNamespace(c, 3000, "baz")

	var inserts []change
	for i := 0; i < 4; i++ {
		ch := change{
			id:   ((i % 2) + 1) * 1000,
			uuid: utils.MustNewUUID().String(),
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
			uuid: utils.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		inserts = append(inserts, ch)
	}

	stream := New(s.TrackedDB(), s.FileNotifier, s.clock, s.logger)
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
		if inserts[idx].id == 2000 {
			namespace = "bar"
		} else if inserts[idx].id == 3000 {
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
	tick := s.setupTimer()
	done := s.expectTick(tick, 1)

	s.insertNamespace(c, 1000, "foo")

	stream := New(s.TrackedDB(), s.FileNotifier, s.clock, s.logger)
	defer workertest.DirtyKill(c, stream)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for timer to fire")
	}

	changes := stream.Changes()

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
		uuid: utils.MustNewUUID().String(),
	}
	s.insertChange(c, first)

	select {
	case change := <-changes:
		c.Fatalf("unexpected change %v", change)
	case <-time.After(witnessChangeDuration):
	}

	expectNotifyBlock(false)

	done = s.expectTick(tick, 1)
	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for timer to fire")
	}

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

func (s *streamSuite) TestReadChangesWithNoChanges(c *gc.C) {
	stream := &Stream{
		db: s.TrackedDB(),
	}

	s.insertNamespace(c, 1000, "foo")

	results, err := stream.readChanges()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 0)
}

func (s *streamSuite) TestReadChangesWithOneChange(c *gc.C) {
	stream := &Stream{
		db: s.TrackedDB(),
	}

	s.insertNamespace(c, 1000, "foo")

	first := change{
		id:   1000,
		uuid: utils.MustNewUUID().String(),
	}
	s.insertChange(c, first)

	results, err := stream.readChanges()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Namespace(), gc.Equals, "foo")
	c.Assert(results[0].ChangedUUID(), gc.Equals, first.uuid)
}

func (s *streamSuite) TestReadChangesWithMultipleSameChange(c *gc.C) {
	stream := &Stream{
		db: s.TrackedDB(),
	}

	s.insertNamespace(c, 1000, "foo")

	uuid := utils.MustNewUUID().String()
	for i := 0; i < 10; i++ {
		ch := change{
			id:   1000,
			uuid: uuid,
		}
		s.insertChange(c, ch)
	}

	results, err := stream.readChanges()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Namespace(), gc.Equals, "foo")
	c.Assert(results[0].ChangedUUID(), gc.Equals, uuid)
}

func (s *streamSuite) TestReadChangesWithMultipleChanges(c *gc.C) {
	stream := &Stream{
		db: s.TrackedDB(),
	}

	s.insertNamespace(c, 1000, "foo")

	changes := make([]change, 10)
	for i := 0; i < 10; i++ {
		ch := change{
			id:   1000,
			uuid: utils.MustNewUUID().String(),
		}
		s.insertChange(c, ch)
		changes[i] = ch
	}

	results, err := stream.readChanges()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 10)
	for i := range results {
		c.Assert(results[i].Namespace(), gc.Equals, "foo")
		c.Assert(results[i].ChangedUUID(), gc.Equals, changes[len(results)-1-i].uuid)
	}
}

func (s *streamSuite) TestReadChangesWithMultipleChangesGroupsCorrectly(c *gc.C) {
	stream := &Stream{
		db: s.TrackedDB(),
	}

	s.insertNamespace(c, 1000, "foo")

	changes := make([]change, 10)
	for i := 0; i < 10; i++ {
		var (
			ch   change
			uuid = utils.MustNewUUID().String()
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
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 10)
	for i := range results {
		c.Assert(results[i].Namespace(), gc.Equals, "foo")
		c.Assert(results[i].ChangedUUID(), gc.Equals, changes[len(results)-1-i].uuid)
	}
}

func (s *streamSuite) TestReadChangesWithMultipleChangesInterweavedGroupsCorrectly(c *gc.C) {
	stream := &Stream{
		db: s.TrackedDB(),
	}

	s.insertNamespace(c, 1000, "foo")
	s.insertNamespace(c, 2000, "bar")

	// Setup for this test is a bit more complicated to ensure that interweaving
	// correctly groups the changes.

	changes := make([]change, 5)

	var (
		uuid0 = utils.MustNewUUID().String()
		uuid1 = utils.MustNewUUID().String()
		uuid2 = utils.MustNewUUID().String()
	)

	{
		ch := change{id: 1000, uuid: uuid0}
		s.insertChangeForType(c, changestream.Create, ch)
		changes[0] = ch
	}
	{
		ch := change{id: 2000, uuid: uuid0}
		s.insertChangeForType(c, changestream.Update, ch)
		changes[1] = ch
	}
	{
		ch := change{id: 1000, uuid: uuid1}
		s.insertChangeForType(c, changestream.Update, ch)
		changes[2] = ch
	}
	{
		ch := change{id: 1000, uuid: uuid1}
		s.insertChangeForType(c, changestream.Update, ch)
		// no witness changed.
	}
	{
		ch := change{id: 2000, uuid: uuid0}
		s.insertChangeForType(c, changestream.Update, ch)
		// no witness changed.
	}
	{
		ch := change{id: 1000, uuid: uuid2}
		s.insertChangeForType(c, changestream.Update, ch)
		changes[3] = ch
	}
	{
		ch := change{id: 1000, uuid: uuid2}
		s.insertChangeForType(c, changestream.Update, ch)
		// no witness changed.
	}
	{
		ch := change{id: 2000, uuid: uuid0}
		s.insertChangeForType(c, changestream.Update, ch)
		// no witness changed.
	}
	{
		ch := change{id: 1000, uuid: uuid1}
		s.insertChangeForType(c, changestream.Create, ch)
		changes[4] = ch
	}

	results, err := stream.readChanges()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 5)

	type changeResults struct {
		changeType changestream.ChangeType
		namespace  string
		uuid       string
	}

	expected := []changeResults{
		{changeType: changestream.Create, namespace: "foo", uuid: uuid1},
		{changeType: changestream.Update, namespace: "bar", uuid: uuid0},
		{changeType: changestream.Update, namespace: "foo", uuid: uuid2},
		{changeType: changestream.Update, namespace: "foo", uuid: uuid1},
		{changeType: changestream.Create, namespace: "foo", uuid: uuid0},
	}

	c.Logf("result %v", results)
	for i := range results {
		c.Logf("expected %v", expected[i])
		c.Assert(results[i].Type(), gc.Equals, expected[i].changeType)
		c.Assert(results[i].Namespace(), gc.Equals, expected[i].namespace)
		c.Assert(results[i].ChangedUUID(), gc.Equals, expected[i].uuid)
	}
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
	s.insertChangeForType(c, 2, changes...)
}

func (s *streamSuite) insertChangeForType(c *gc.C, changeType changestream.ChangeType, changes ...change) {
	q := `
INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid)
VALUES (?, ?, ?)
`[1:]

	tx, err := s.DB().Begin()
	c.Assert(err, jc.ErrorIsNil)

	stmt, err := tx.Prepare(q)
	c.Assert(err, jc.ErrorIsNil)

	for _, v := range changes {
		c.Logf("Executing insert change: edit-type: %d, %v %v", changeType, v.id, v.uuid)
		_, err = stmt.Exec(changeType, v.id, v.uuid)
		c.Assert(err, jc.ErrorIsNil)
	}

	c.Logf("Committing insert change")
	err = tx.Commit()
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("Committed insert change")
}
