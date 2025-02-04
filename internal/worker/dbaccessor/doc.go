// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package dbaccessor defines workers that create and operate the Dqlite cluster
// and the individual Dqlite databases, as follows:

// (1) the dbWorker (aka "the DBAccessor worker"; see worker.go) uses
// internal/database and database/sql to set up and coordinate the Dqlite
// cluster, and to create individual Dqlite databases.
//
// (2) the trackedDBWorker (see tracker.go) exposes database/sql provides
// access to the individual databases and manages the lifecycle of the database
// connections.

// the transaction runners it creates present the database/sql functions to
// consumers (they open the database connection and give the ability to run
// transactions to third parties)
//
// Dqlite cluster genesis
//
// When a new controller node comes into existence, either through bootstrap or
// HA, a configuration file is created on disk. If it's on the bootstrap node (e.g., machine 0), this file
// will have contents defining the Dqlite instance. On any other controller node
// (established through HA), this file will have no contents, which will be an
// indication that the Dqlite instance on this node must join the Dqlite
// cluster.
//
// Individual database
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
// presents database/sqlfunctions

// the Dqlite cluster < the Dqlite database < database/sql
// (https://pkg.go.dev/database/sql; helps you access the databases: (1) gives
// you transactions and the ability to query and write data via a driver and (2)
// the ability to operate clusters) < the internal/worker/dbaccessor package //
// < internal/worker/changestream package < internal/worker/domainservices
// package

// These workers are (1) the dbWorker (aka "the DBAccessor worker"; see
// worker.go), which sets up and coordinates the Dqlite cluster, and (2) the
// trackedDBWorker (see tracker.go), which provides access to the individual
// databases (one for the controller and one for each model) and manages the
// lifecycle of the database connections.

// The dbaccessor packages uses internal/database to set up the cluster and create individual
// dqlite dbs

// The dbaccesor package presents database/sqlfunctions to

// the transaction runners it creates present the database/sql functions to
// consumers (they open the database connection and give the ability to run
// transactions to third parties)
//
// (2) offers the ability to run transactions
//
// After it’s set up and clustered Dqlite, one of its outputs to other workers
// is a DBGetter interface which has a single method, GetDB, which those workers
// can call. The result of calling that method, depending on which argument you
// pass, is a transaction runner for a database, where the database can be for
// the controller or a model. (The method looks like this: `GetDB(namespace
// string) (database.TxnRunner, error) }`, where the namespace string is either
// ‘controller’ or a model UUID.) This will also start other workers, TrackedDB,
// which will cache the database (one per db to be cached) and wrap around the
// db. It’s these workers that monitor the health of the db, implement retries
// for transactions, if required, and periodically ping the db to see if we’re
// still connected to it and it’s still responding (if it’s down, it’ll get it
// up again). The reason for this layered approach is so the third-party user
// doesn’t have to be concerned with the low-level details of the db – we handle
// that and they only interact with the transaction runner. node.go presents the
// NodeManager type to the dbaccessor
//
// DBAccessor watches the controller config on disk to see if it changes. Upon
// bootstrap, we read the controller config, if it’s there. If it’s not, we
// enter into a single controller mode. In the case that there is a single local
// cloud address (for machines), we bind Dqlite to that address. On K8s we bind
// to localhost.

// The stuff in internal/database (esp. node.go) is called, among other places,
// by the dbaccessor packages
// (https://github.com/juju/juju/tree/main/internal/worker/dbaccessor ) which
// sets up the Dqlite server and also offers the ability to run transactions:
// After it’s set up and clustered Dqlite, one of its outputs to other workers
// is a DBGetter interface which has a single method, GetDB, which those workers
// can call.  The result of calling that method, depending on which argument you
// pass, is a transaction runner for a database, where the database can be for
// the controller or a model. (The method looks like this: `GetDB(namespace
// string) (database.TxnRunner, error) }`, where the namespace string is either
// ‘controller’ or a model UUID.) This will also start other workers, TrackedDB,
// which will cache the database (one per db to be cached) and wrap around the
// db. It’s these workers that monitor the health of the db, implement retries
// for transactions, if required, and periodically ping the db to see if we’re
// still connected to it and it’s still responding (if it’s down, it’ll get it
// up again). The reason for this layered approach is so the third-party user
// doesn’t have to be concerned with the low-level details of the db – we handle
// that and they only interact with the transaction runner.
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
