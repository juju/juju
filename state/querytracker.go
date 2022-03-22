// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"runtime/debug"
	"strings"
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
// track the database queries made. The query is counted if
// the method specified is found in the traceback. It is currently
// just using a string.Contains check. We may want to extend this
// functionality at some state to use regex, or support multiple
// matches.
func (s *State) TrackQueries(method string) QueryTracker {
	tracker := &queryTracker{method: method}
	s.database.(*database).setTracker(tracker)
	return tracker
}

type queryTracker struct {
	method  string
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

	traceback := string(debug.Stack())
	if strings.Contains(traceback, q.method) {
		q.queries = append(q.queries, QueryDetails{
			Type:           "read",
			CollectionName: collectionName,
			Query:          query,
			Traceback:      traceback,
		})
	}
}
