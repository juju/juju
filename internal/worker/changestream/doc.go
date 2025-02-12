// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package changestream adds the ability to create watchers to the transaction
// runners created by the internal/worker/dbaccessor package. These databases
// are then used by the internal/worker/domainservices package.
//
// How do databases become watchable?
//
// This happens through a worker that manages the lifecycle of the
// internal/changestream/stream worker and of the
// internal/changestream/eventmultiplexer worker as the former polls the
// database for changes and as the latter forwards them as events to the
// appropriate event subscriber:
//
//  ┌──────────┐          ┌──────────┐
//  │          │          │          │
//  │          │          │          │
//  │  Dqlite  ◄──────────┤  stream  │
//  │    DB    │          │          │
//  │          │          │          │
//  └──────────┘          └─────┬────┘
//                              │
//                      ┌───────▼───────┐
//                      │               │
//                      │               │
//                      │   event mux   │
//                      │               │
//                      │               │
//                      └───────┬───────┘
//                              │
//              ┌──────────┬────┴─────┬──────────┐
//              │          │          │          │
//          ┌───▼───┐  ┌───▼───┐  ┌───▼───┐  ┌───▼───┐
//          │       │  │       │  │       │  │       │
//          │ sub 0 │  │ sub 1 │  │ sub 2 │  │ sub N │
//          │       │  │       │  │       │  │       │
//          └───────┘  └───────┘  └───────┘  └───────┘
//

package changestream
