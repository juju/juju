// Copyright 2023 Canonical Ltd. Licensed under the AGPLv3, see LICENCE file for
// details.

// Package dbaccessor defines workers that create and operate the Dqlite cluster
// and the individual Dqlite databases and the connections to them. These
// databases are then made watchable by the internal/worker/changestream package
// and used by the internal/worker/domainservices package.
//
// # dbaccessor.dbWorker
//
// This worker uses internal/database and database/sql to set up and coordinate
// the Dqlite cluster, and to create individual Dqlite databases. Once that's
// done, this worker exports a DBGetter interface to the dependency engine. This
// interface has a single method, GetDB, that workers can call to obtain a
// TxnRunner for a database.
//
// Dqlite cluster genesis
//
// When a new controller node comes into existence, either through bootstrap or
// HA, a configuration file is created on disk. If it's on the bootstrap node
// (e.g., machine 0), this file will have contents defining the Dqlite instance.
// On any other controller node (established through HA), this file will have no
// contents, which will be an indication to dbaccessor.dbWorker that the Dqlite
// instance on this node must join the Dqlite cluster.
//
// Individual Dqlite databases
//
// There is an individual database for the controller and one for each model.
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
//
// # dbaccessor.trackedDBWorker
//
// This worker uses Dqlite to open a database that exposes database/sql
// functionality in order to provide individual databases, where each database
// is wrapped by a TxnRunner to retry transactions in the face of Dqlite errors.
//
// The worker also manages the lifecycle of database connections via health
// pings: If a database connection is temporarily lost, the trackedDBWorker will
// attempt to reconnect to the database. If the connection is permanently lost,
// the trackedDBWorker will terminate and the dbWorker start a new one.
//
//  ┌───────────────────┐
//  │                   │
//  │   Txn Runner      │
//  │                   ├──────────┐
//  │  ┌─────────────┐  │          │
//  │  │             │  │        PING
//  │  │             │  │          │
//  │  │  Dqlite DB  ◄──┼──────────┘
//  │  │             │  │
//  │  │             │  │
//  │  └─────────────┘  │
//  │                   │
//  └───────────────────┘
//
// The reason for having a dbWorker as well as a trackeDBWorker, so, a layered
// approach, is so that the Juju developer doesn’t have to be concerned with the
// low-level details of a transaction – the TxnRunner takes care of all of that.

package dbaccessor
