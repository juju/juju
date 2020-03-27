// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"runtime/debug"
	"sync"
)

// QueryTracker provides a way for tests to determine how many
// database queries have been made, and who made them.
type QueryTracker interface {
	Reset()
	ListQueries() []QueryDetails
	ReadCount() int
	// TODO: implement the write tracking.
	// WriteCount() int
}

// QueryDetails is a POD type recording the database query
// and who made it.
type QueryDetails struct {
	Type           string // read or write
	CollectionName string
	Query          interface{}
	Traceback      string
}

// TrackQueries allows tests to turn on a mechanism to count and
// track the database queries made.
func (s *State) TrackQueries() QueryTracker {
	tracker := &queryTracker{}
	s.database.(*database).setTracker(tracker)
	return tracker
}

type queryTracker struct {
	mu      sync.Mutex
	queries []QueryDetails
}

// Reset clears out all the current reads and writes.
func (q *queryTracker) Reset() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.queries = nil
}

// ListQueries returns the list of all queries that have been
// done since start or reset.
func (q *queryTracker) ListQueries() []QueryDetails {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.queries
}

// ReadCount returns the number of read queries that have been
// done since start or reset.
func (q *queryTracker) ReadCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	count := 0
	for _, query := range q.queries {
		if query.Type == "read" {
			count++
		}
	}
	return count
}

// WriteCount returns the number of write queries that have been
// done since start or reset.
func (q *queryTracker) WriteCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	count := 0
	for _, query := range q.queries {
		if query.Type == "write" {
			count++
		}
	}
	return count
}

// TrackRead records the read query against the collection specified
// and where the call came from.
func (q *queryTracker) TrackRead(collectionName string, query interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.queries = append(q.queries, QueryDetails{
		Type:           "read",
		CollectionName: collectionName,
		Query:          query,
		Traceback:      string(debug.Stack()),
	})
}
