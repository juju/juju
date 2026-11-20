// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

// MergeLastWins combines current and newChanges by key, keeping only the
// final occurrence of each key at its final chronological position (an
// updated key moves to the tail after earlier events for other keys).
// A merged batch therefore carries at most one entry per key and the
// latest value wins.
//
// Agent-side and controller-side trigger watchers must use this single
// implementation so their batching semantics cannot drift apart.
func MergeLastWins[T any, K comparable](current, newChanges []T, key func(T) K) []T {
	combined := make([]T, 0, len(current)+len(newChanges))
	combined = append(combined, current...)
	combined = append(combined, newChanges...)
	lastIdx := make(map[K]int, len(combined))
	for i, c := range combined {
		lastIdx[key(c)] = i
	}
	result := make([]T, 0, len(lastIdx))
	for i, c := range combined {
		if lastIdx[key(c)] == i {
			result = append(result, c)
		}
	}
	return result
}
