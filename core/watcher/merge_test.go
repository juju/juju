// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher"
)

type mergeSuite struct{}

var _ = gc.Suite(&mergeSuite{})

// TestMergeLastWinsEmpty ensures merging nothing yields nothing.
func (*mergeSuite) TestMergeLastWinsEmpty(c *gc.C) {
	c.Check(watcher.MergeLastWins[int, string](nil, nil, func(int) string {
		return ""
	}), gc.HasLen, 0)
}

// TestMergeLastWinsMovesUpdatedKeysToTail ensures updated keys keep only
// the last value, repositioned at the tail after earlier events for other
// keys.
func (*mergeSuite) TestMergeLastWinsMovesUpdatedKeysToTail(c *gc.C) {
	type change struct {
		id   string
		next time.Time
	}
	now := time.Now().UTC()
	key := func(ch change) string { return ch.id }

	got := watcher.MergeLastWins(
		[]change{
			{id: "a", next: now.Add(time.Hour)},
			{id: "b", next: now.Add(2 * time.Hour)},
		},
		[]change{
			{id: "c", next: now.Add(3 * time.Hour)},
			{id: "a"},
		},
		key,
	)
	c.Assert(got, gc.HasLen, 3)
	c.Check(got[0].id, gc.Equals, "b")
	c.Check(got[0].next, gc.Equals, now.Add(2*time.Hour))
	c.Check(got[1].id, gc.Equals, "c")
	c.Check(got[2].id, gc.Equals, "a")
	c.Check(got[2].next.IsZero(), jc.IsTrue)
}

// TestMergeLastWinsDeduplicatesWithinNewChanges ensures duplicates within
// the new batch itself collapse to the last occurrence, repositioned at
// the tail.
func (*mergeSuite) TestMergeLastWinsDeduplicatesWithinNewChanges(c *gc.C) {
	type change struct {
		id   string
		next time.Time
	}
	now := time.Now().UTC()
	key := func(ch change) string { return ch.id }

	got := watcher.MergeLastWins(nil, []change{
		{id: "a", next: now},
		{id: "b", next: now.Add(time.Hour)},
		{id: "a", next: now.Add(2 * time.Hour)},
	}, key)
	c.Assert(got, gc.HasLen, 2)
	c.Check(got[0].id, gc.Equals, "b")
	c.Check(got[0].next, gc.Equals, now.Add(time.Hour))
	c.Check(got[1].id, gc.Equals, "a")
	c.Check(got[1].next, gc.Equals, now.Add(2*time.Hour))
}
