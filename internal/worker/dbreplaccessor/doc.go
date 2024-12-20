// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package dprelaccessor is a worker that provides access to the SQLite database
// via access to the read-only Dqlite cluster. The worker does not participate
// in the Dqlite cluster, but instead connects to the Dqlite cluster to read
// data from the database. Queries still perform reads and writes to the SQLite
// database via the TxnRunner.
//
//  ┌─────────────────┐
//  │                 │
//  │                 │
//  │  Controller DB  │
//  │                 │
//  │                 │
//  └─────────────────┘
//
//  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
//  │              │ │              │ │              │
//  │              │ │              │ │              │
//  │  Model 1 DB  │ │  Model 2 DB  │ │  Model N DB  │
//  │              │ │              │ │              │
//  │              │ │              │ │              │
//  └──────────────┘ └──────────────┘ └──────────────┘
//
// Each database is wrapped via a TrackedDBWorker, which is responsible for
// managing the lifecycle of the database connection. The dbreplaccessor is not
// intended to be resilient to database connection loss. If the connection to
// the Dqlite cluster is lost, the dbreplaccessor will terminate all workers and
// the worker manager will restart the worker.

package dbreplaccessor
