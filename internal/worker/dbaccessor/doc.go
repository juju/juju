// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package DBAccessor is a worker that provides access to the Juju database.
// It is responsible for accessing the individual databases. One database per
// model and an additional model for a controller.
//
//	┌─────────────────┐
//	│                 │
//	│                 │
//	│  Controller DB  │
//	│                 │
//	│                 │
//	└─────────────────┘
//
//	┌──────────────┐ ┌──────────────┐ ┌──────────────┐
//	│              │ │              │ │              │
//	│              │ │              │ │              │
//	│  Model 1 DB  │ │  Model 2 DB  │ │  Model N DB  │
//	│              │ │              │ │              │
//	│              │ │              │ │              │
//	└──────────────┘ └──────────────┘ └──────────────┘
//
// Each database is wrapped via a TrackedDBWorker, which is responsible for
// managing the lifecycle of the database connection. If a database connection
// is temporarily lost, the TrackedDBWorker will attempt to reconnect to the
// database. If the connection is permanently lost, the TrackedDBWorker will
// terminate the DBAccessor worker and a new one will be started by the worker
// manager.
//
//	┌───────────────────┐
//	│                   │
//	│   Txn Runner      │
//	│                   ├──────────┐
//	│  ┌─────────────┐  │          │
//	│  │             │  │        PING
//	│  │             │  │          │
//	│  │  Dqlite DB  ◄──┼──────────┘
//	│  │             │  │
//	│  │             │  │
//	│  └─────────────┘  │
//	│                   │
//	└───────────────────┘
//
// The DBAccessor is officially the only worker that should be accessing the
// database directly. All other workers, including the apiserver should be
// accessing the database via the domain services.

package dbaccessor
