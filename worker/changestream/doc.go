// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Changestream is a worker that handles stream changes from the database as
// triggers. The changestream worker is responsible for managing the lifecycle
// of a stream and event multiplexer worker. The changestream worker encompasses
// the following components:
//
//	┌──────────┐          ┌──────────┐
//	│          │          │          │
//	│          │          │          │
//	│  Dqlite  ◄──────────┤  stream  │
//	│    DB    │          │          │
//	│          │          │          │
//	└──────────┘          └─────┬────┘
//	                            │
//	                    ┌───────▼───────┐
//	                    │               │
//	                    │               │
//	                    │   event mux   │
//	                    │               │
//	                    │               │
//	                    └───────┬───────┘
//	                            │
//	            ┌──────────┬────┴─────┬──────────┐
//	            │          │          │          │
//	        ┌───▼───┐  ┌───▼───┐  ┌───▼───┐  ┌───▼───┐
//	        │       │  │       │  │       │  │       │
//	        │ sub 0 │  │ sub 1 │  │ sub 2 │  │ sub N │
//	        │       │  │       │  │       │  │       │
//	        └───────┘  └───────┘  └───────┘  └───────┘
//
// In its simplest form the stream worker will poll the database change log for
// changes and send them to the event multiplexer. The event multiplexer will
// then send the changes to the appropriate subscription.
//
// To aid with performance the stream worker will coalesce changes from the
// database change log. So if a change from the same namespace and changed value
// is received within a polling request, then the changes will be coalesced into
// a single change.
//
// For example, if the following changes are received from the database change
// log, then the first change (NS 1, UUID A) and the third change (NS 1, UUID A)
// will be coalesced into a single change. You'll only see the last change.
//
//	┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐
//	│         │ │         │ │         │ │         │
//	│ NS   1  │ │ NS   1  │ │ NS   1  │ │ NS   1  │
//	│ UUID A  │ │ UUID B  │ │ UUID A  │ │ UUID C  │
//	│         │ │         │ │         │ │         │
//	└─────────┘ └─────────┘ └─────────┘ └─────────┘
//
//	┌─────────┐ ┌─────────┐ ┌─────────┐
//	│         │ │         │ │         │
//	│ NS   1  │ │ NS   1  │ │ NS   1  │
//	│ UUID B  │ │ UUID A  │ │ UUID C  │
//	│         │ │         │ │         │
//	└─────────┘ └─────────┘ └─────────┘
//
// No additional information about the change is provided in the event, other
// than:
//
//   - The change type (CREATE, UPDATE, DELETE)
//   - The namespace (generally the table name, but not guaranteed)
//   - The changed (this could be the primary key of the row that changed)
//
// It's not useful to provide additional information about the change, because
// in an eventual system, the change may be considered stale by the time it
// reaches the subscriber. The subscriber will need to query the database to
// retrieve the latest information about the change. They may already have
// witnessed the change so may be ignored at that point.
//
// The event multiplexer will send the changes to the appropriate subscription.
// This involves grouping changes into terms. Terms are a set of changes that
// are sent to the subscriber in a single batch. Terms are used to ensure that
// that there is a consistent view of the database at the subscriber.
// The event multiplexer will wait for all the terms to be read by subscribers
// before attempting to send the next term. All terms will be sent in a
// async manner, allowing all subscribers to work on the same term at the same
// time. Once a term is marked as done, then and only then is another term
// expected to be worked on.
//
// Terms are expected to be consumed as fast as possible. If a term can not be
// processed or is taking too long to consume by a subscriber then the
// subscriber will be unsubscribed from the next term. This is to ensure that
// the event multiplexer does not get blocked by a slow subscriber.
//
//	┌───────────┐ ┌───────────┐ ┌───────────────────────────┐ ┌───────────┐
//	│           │ │           │ │                           │ │           │
//	│  TERM 1   │ │  TERM 2   │ │  TERM 3                   │ │  TERM 4   │
//	│           │ │           │ │                           │ │           │
//	└───────────┘ └───────────┘ └───────────────────────────┘ └───────────┘
//
// In the above example, the event multiplexer will wait for all subscribers to
// consume TERM 1 before sending TERM 2. If a subscriber is slow to consume TERM
// 3, then the subscriber will be unsubscribed from subsequent terms. The
// subscriber will be notified of the unsubscribe event. They can then decide
// to either resubscribe or start from a fresh state and try and keep up.
// This model will ensure that one subscriber doesn't attempt to bring down the
// entire system.
//
// If a subscriber can't consume or process a term fast enough, it is then
// up to the subscriber to delegate that to another channel or worker in an
// async fashion, thus allowing the subscriber to continue to consume terms.

package changestream
