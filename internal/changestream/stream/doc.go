// Copyright 2025 Canonical Ltd. Licensed under the AGPLv3, see LICENCE file for
// details.
//
// Package stream defines a worker that polls the database changelog for changes
// and sends them in the form of an event to the
// internal/changestream/eventmultiplexer worker.
//
// Change information
//
// When sending information about a change, the stream worker includes the
// following:
//
//   - The change type (CREATE, UPDATE, DELETE)
//   - The namespace (generally the table name, but not guaranteed)
//   - The changed (this could be the primary key of the row that changed)
//
// Note: This information all amounts to a notification that something has
// happened. The reason no specifics about what exactly has happened are
// included is because, as in every eventually consistent system, that
// information can easily get stale. To retrieve the latest information, each
// subscriber must query the database when they receive the notification.
//
// Change grouping
//
// The stream worker groups changes into batches known as 'terms'. For example,
// below (where NS = namespace), 4 changes make up a term:
//
//  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐
//  │         │ │         │ │         │ │         │
//  │ NS   1  │ │ NS   1  │ │ NS   1  │ │ NS   1  │
//  │ UUID A  │ │ UUID B  │ │ UUID A  │ │ UUID C  │
//  │         │ │         │ │         │ │         │
//  └─────────┘ └─────────┘ └─────────┘ └─────────┘
//
// Change coalescing
//
// To aid performance, if polling the database retrieves multiple changes with
// the same namespace and UUID, the stream worker will coalesce those changes
// into a single change.
//
// For example, if the polling request reveals 4 changes where the 1st and the
// 3rd have the same namespace and UUID (NS 1, UUID A), as below:
//
//  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐
//  │         │ │         │ │         │ │         │
//  │ NS   1  │ │ NS   1  │ │ NS   1  │ │ NS   1  │
//  │ UUID A  │ │ UUID B  │ │ UUID A  │ │ UUID C  │
//  │         │ │         │ │         │ │         │
//  └─────────┘ └─────────┘ └─────────┘ └─────────┘
//
// the stream worker will coalesce those changes into the last change, so the
// changes it will send on become:
//
//  ┌─────────┐ ┌─────────┐ ┌─────────┐
//  │         │ │         │ │         │
//  │ NS   1  │ │ NS   1  │ │ NS   1  │
//  │ UUID B  │ │ UUID A  │ │ UUID C  │
//  │         │ │         │ │         │
//  └─────────┘ └─────────┘ └─────────┘
//

package stream
