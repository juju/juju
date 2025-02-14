// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package changestream equips the transaction runners created by the
// internal/worker/dbaccessor package with the ability to watch for specific
// changes to data.
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
