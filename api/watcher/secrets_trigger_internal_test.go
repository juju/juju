// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

type secretsTriggerWatcherMergeSuite struct{}

var _ = gc.Suite(&secretsTriggerWatcherMergeSuite{})

// TestMergeKeepsRevisions ensures that expiry triggers for distinct
// revisions of the same secret do not clobber each other.
func (s *secretsTriggerWatcherMergeSuite) TestMergeKeepsRevisions(c *gc.C) {
	uri := secrets.NewURI()
	now := time.Now()
	w := &secretsTriggerWatcher{}

	changes := w.mergeChanges(
		[]corewatcher.SecretTriggerChange{{
			URI:             uri,
			Revision:        1,
			NextTriggerTime: now,
		}},
		[]corewatcher.SecretTriggerChange{{
			URI:             uri,
			Revision:        2,
			NextTriggerTime: now.Add(time.Hour),
		}},
	)
	c.Assert(changes, gc.HasLen, 2)
	got := make(map[int]time.Time)
	for _, ch := range changes {
		got[ch.Revision] = ch.NextTriggerTime
	}
	c.Check(got[1].Equal(now), jc.IsTrue)
	c.Check(got[2].Equal(now.Add(time.Hour)), jc.IsTrue)
}

// TestMergeReplacesSameRevision ensures that a later trigger for the same
// secret revision replaces the earlier one.
func (s *secretsTriggerWatcherMergeSuite) TestMergeReplacesSameRevision(c *gc.C) {
	uri := secrets.NewURI()
	now := time.Now()
	w := &secretsTriggerWatcher{}

	changes := w.mergeChanges(
		[]corewatcher.SecretTriggerChange{{
			URI:             uri,
			Revision:        1,
			NextTriggerTime: now,
		}},
		[]corewatcher.SecretTriggerChange{{
			URI:             uri,
			Revision:        1,
			NextTriggerTime: now.Add(time.Hour),
		}},
	)
	c.Assert(changes, gc.HasLen, 1)
	c.Check(changes[0].NextTriggerTime.Equal(now.Add(time.Hour)), jc.IsTrue)
}

// TestCopyChangesDropsInvalidURI ensures invalid URIs are dropped rather
// than delivered with a nil URI.
func (s *secretsTriggerWatcherMergeSuite) TestCopyChangesDropsInvalidURI(c *gc.C) {
	uri := secrets.NewURI()
	now := time.Now().UTC()
	w := &secretsTriggerWatcher{}

	changes := w.copyChanges([]params.SecretTriggerChange{
		{
			URI:             "not-a-valid-uri",
			Revision:        1,
			NextTriggerTime: now,
		},
		{
			URI:             uri.String(),
			Revision:        2,
			NextTriggerTime: now.Add(time.Hour),
		},
	})
	c.Assert(changes, gc.HasLen, 1)
	c.Check(changes[0].URI, gc.NotNil)
	c.Check(changes[0].URI.ID, gc.Equals, uri.ID)
	c.Check(changes[0].Revision, gc.Equals, 2)
	c.Check(changes[0].NextTriggerTime.Equal(now.Add(time.Hour)), jc.IsTrue)
}

// TestMergePreservesOrder ensures distinct keys keep their relative order
// and an updated key is repositioned at the tail with only its latest
// value retained.
func (s *secretsTriggerWatcherMergeSuite) TestMergePreservesOrder(c *gc.C) {
	uri1 := secrets.NewURI()
	uri2 := secrets.NewURI()
	now := time.Now().UTC()
	w := &secretsTriggerWatcher{}

	batch1 := w.copyChanges([]params.SecretTriggerChange{
		{URI: uri1.String(), Revision: 0},
		{URI: uri2.String(), Revision: 2, NextTriggerTime: now.Add(time.Hour)},
	})
	batch2 := w.copyChanges([]params.SecretTriggerChange{
		{URI: uri1.String(), Revision: 2, NextTriggerTime: now.Add(2 * time.Hour)},
		{URI: uri2.String(), Revision: 0},
	})

	merged := w.mergeChanges(batch1, batch2)
	c.Assert(merged, gc.HasLen, 4)
	want := []struct {
		id  string
		rev int
	}{
		{uri1.ID, 0},
		{uri2.ID, 2},
		{uri1.ID, 2},
		{uri2.ID, 0},
	}
	for i, exp := range want {
		c.Check(merged[i].URI.ID, gc.Equals, exp.id)
		c.Check(merged[i].Revision, gc.Equals, exp.rev)
	}

	// Overwriting an existing key moves it to the tail and retains only
	// the latest value.
	batch3 := w.copyChanges([]params.SecretTriggerChange{
		{URI: uri1.String(), Revision: 0, NextTriggerTime: now.Add(3 * time.Hour)},
	})
	merged = w.mergeChanges(merged, batch3)
	c.Assert(merged, gc.HasLen, 4)
	last := merged[len(merged)-1]
	c.Check(last.URI.ID, gc.Equals, uri1.ID)
	c.Check(last.Revision, gc.Equals, 0)
	c.Check(last.NextTriggerTime.Equal(now.Add(3*time.Hour)), jc.IsTrue)
}

// TestMergeDeleteAndReenableLastWins ensures the last event per key wins,
// in both directions across batches.
func (s *secretsTriggerWatcherMergeSuite) TestMergeDeleteAndReenableLastWins(c *gc.C) {
	uri1 := secrets.NewURI()
	uri2 := secrets.NewURI()
	now := time.Now().UTC()
	w := &secretsTriggerWatcher{}

	// uri1: delete, then re-enable across batches.
	// uri2: enable, then delete across batches.
	b1 := w.copyChanges([]params.SecretTriggerChange{
		{URI: uri1.String()},
		{URI: uri2.String(), NextTriggerTime: now.Add(time.Hour)},
	})
	b2 := w.copyChanges([]params.SecretTriggerChange{
		{URI: uri1.String(), NextTriggerTime: now.Add(2 * time.Hour)},
		{URI: uri2.String()},
	})
	merged := w.mergeChanges(b1, b2)

	c.Assert(merged, gc.HasLen, 2)
	byID := make(map[string]corewatcher.SecretTriggerChange, 2)
	for _, ch := range merged {
		byID[ch.URI.ID] = ch
	}
	c.Check(byID[uri1.ID].NextTriggerTime.Equal(now.Add(2*time.Hour)), jc.IsTrue)
	c.Check(byID[uri2.ID].NextTriggerTime.IsZero(), jc.IsTrue)
}

type secretBackendRotateWatcherMergeSuite struct{}

var _ = gc.Suite(&secretBackendRotateWatcherMergeSuite{})

// TestMergePreservesOrder ensures distinct backends keep their relative
// order and a deletion for one backend lands at the tail.
func (s *secretBackendRotateWatcherMergeSuite) TestMergePreservesOrder(c *gc.C) {
	now := time.Now().UTC()
	w := &secretBackendRotateWatcher{}

	merged := w.mergeChanges(
		[]corewatcher.SecretBackendRotateChange{
			{ID: "b1", Name: "b1", NextTriggerTime: now.Add(time.Hour)},
			{ID: "b2", Name: "b2", NextTriggerTime: now.Add(2 * time.Hour)},
		},
		[]corewatcher.SecretBackendRotateChange{
			{ID: "b1", Name: "b1"},
		},
	)
	c.Assert(merged, gc.HasLen, 2)
	c.Check(merged[0].ID, gc.Equals, "b2")
	c.Check(merged[1].ID, gc.Equals, "b1")
	c.Check(merged[1].NextTriggerTime.IsZero(), jc.IsTrue)
}
