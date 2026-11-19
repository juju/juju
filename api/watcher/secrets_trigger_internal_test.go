// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"reflect"
	"testing"
	"time"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

func TestSecretsTriggerMergeKeepsRevisions(t *testing.T) {
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
	if len(changes) != 2 {
		t.Fatalf("got %d changes, want 2: %#v", len(changes), changes)
	}
	got := make(map[int]time.Time)
	for _, ch := range changes {
		got[ch.Revision] = ch.NextTriggerTime
	}
	if !got[1].Equal(now) {
		t.Fatalf("revision 1 trigger = %s, want %s", got[1], now)
	}
	if !got[2].Equal(now.Add(time.Hour)) {
		t.Fatalf("revision 2 trigger = %s, want %s", got[2], now.Add(time.Hour))
	}
}

func TestSecretsTriggerMergeReplacesSameRevision(t *testing.T) {
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
	if len(changes) != 1 {
		t.Fatalf("got %d changes, want 1: %#v", len(changes), changes)
	}
	if !changes[0].NextTriggerTime.Equal(now.Add(time.Hour)) {
		t.Fatalf("trigger = %s, want %s", changes[0].NextTriggerTime, now.Add(time.Hour))
	}
}

func TestSecretsTriggerCopyChangesDropsInvalidURI(t *testing.T) {
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
	if len(changes) != 1 {
		t.Fatalf("got %d changes, want 1: %#v", len(changes), changes)
	}
	if changes[0].URI == nil || changes[0].URI.ID != uri.ID {
		t.Fatalf("got URI %v, want %s", changes[0].URI, uri.ID)
	}
	if changes[0].Revision != 2 {
		t.Fatalf("got revision %d, want 2", changes[0].Revision)
	}
	if !changes[0].NextTriggerTime.Equal(now.Add(time.Hour)) {
		t.Fatalf("got trigger = %s, want %s", changes[0].NextTriggerTime, now.Add(time.Hour))
	}
}

func TestSecretsTriggerMergePreservesOrder(t *testing.T) {
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
	if len(merged) != 4 {
		t.Fatalf("got %d changes, want 4: %#v", len(merged), merged)
	}
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
		if merged[i].URI.ID != exp.id || merged[i].Revision != exp.rev {
			t.Fatalf("merged[%d] = %#v, want id=%s rev=%d", i, merged[i], exp.id, exp.rev)
		}
	}

	// Overwriting an existing key moves it to the tail and retains only the latest.
	batch3 := w.copyChanges([]params.SecretTriggerChange{
		{URI: uri1.String(), Revision: 0, NextTriggerTime: now.Add(3 * time.Hour)},
	})
	merged = w.mergeChanges(merged, batch3)
	if len(merged) != 4 {
		t.Fatalf("got %d changes, want 4: %#v", len(merged), merged)
	}
	last := merged[len(merged)-1]
	if last.URI.ID != uri1.ID || last.Revision != 0 || !last.NextTriggerTime.Equal(now.Add(3*time.Hour)) {
		t.Fatalf("last = %#v, want id=%s rev=0 time=%s", last, uri1.ID, now.Add(3*time.Hour))
	}
}

func TestSecretBackendRotateMergePreservesOrder(t *testing.T) {
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
	if len(merged) != 2 {
		t.Fatalf("got %d changes, want 2: %#v", len(merged), merged)
	}
	if merged[0].ID != "b2" || merged[1].ID != "b1" || !merged[1].NextTriggerTime.IsZero() {
		t.Fatalf("unexpected merged order/values: %#v", merged)
	}
}

func TestSecretsTriggerMergeDeleteAndReenableLastWins(t *testing.T) {
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

	if len(merged) != 2 {
		t.Fatalf("got %d merged entries, want 2: %#v", len(merged), merged)
	}
	byID := make(map[string]corewatcher.SecretTriggerChange, 2)
	for _, ch := range merged {
		byID[ch.URI.ID] = ch
	}
	if got := byID[uri1.ID]; !got.NextTriggerTime.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("uri1 = %#v, want scheduled at %s", got, now.Add(2*time.Hour))
	}
	if got := byID[uri2.ID]; !got.NextTriggerTime.IsZero() {
		t.Fatalf("uri2 = %#v, want delete", got)
	}
}

func TestMergeOrderedTriggerChanges(t *testing.T) {
	type testTriggerChange struct {
		id   string
		next time.Time
	}
	now := time.Now().UTC()
	key := func(ch testTriggerChange) string { return ch.id }

	if got := mergeOrderedTriggerChanges[testTriggerChange, string](nil, nil, key); len(got) != 0 {
		t.Fatalf("got %d entries for nil inputs, want 0: %#v", len(got), got)
	}

	// Updated keys move to their final chronological position at the tail.
	got := mergeOrderedTriggerChanges(
		[]testTriggerChange{
			{id: "a", next: now.Add(time.Hour)},
			{id: "b", next: now.Add(2 * time.Hour)},
		},
		[]testTriggerChange{
			{id: "c", next: now.Add(3 * time.Hour)},
			{id: "a"},
		},
		key,
	)
	want := []testTriggerChange{
		{id: "b", next: now.Add(2 * time.Hour)},
		{id: "c", next: now.Add(3 * time.Hour)},
		{id: "a"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
