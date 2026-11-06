// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package schema contains the schema definitions for all the domains.
// The schema is broken down into a controller (global) namespace and a set of
// model namespaces. Each domain package wraps a set of workflows
// that can affect either the controller or model namespaces. It's generally
// a good idea not to mix the two, because of a lack atomicity across two
// different databases (even if attempting to use ATTACH DATABASE in SQLite)
// can cause inconsistencies or additional complexity.
//
// The schema is a set of DDL SQL statements for controller and models. They're
// independent from each other. Each set of schema is executed in a single
// transaction, and any error will cause the transaction to be rolled back.
//
// Schema triggers are a set of SQL trigger statements that are executed after
// a row is inserted, updated, or deleted. They're used to drive the watchers
// and the change stream in a generic way. All triggers should be first
// implemented as generic generated triggers, unless there's a good reason to
// implement them as a logical custom trigger.
//
// # Schema evolution
//
// Controllers support in-place upgrades only within the same major.minor
// version. Moving models to a different minor or major version requires a
// separately bootstrapped destination controller. Bootstrap creates a new
// controller database; the source controller database is not reused or upgraded.
//
// Model migration imports exported data into a new model database using the
// destination schema, transforming versioned payloads where required:
//
//	Source model DB -> export -> transform -> destination model DB
//
// Migration does not apply the destination schema to the source model database.
// See github.com/juju/juju/domain/controllerupgrader for controller upgrade
// restrictions and github.com/juju/juju/domain/modelimport for payload transforms.
//
// Base schema files may change only when no supported in-place upgrade depends
// on their existing contents. For supported patch-version upgrades, preserve
// applied schema files and their hash chain, and register incremental changes
// in controllerPostPatchFilesByVersion or modelPostPatchFilesByVersion. The
// cross-minor migration path does not justify rewriting an applied schema file
// within a released major.minor series.
//
// Lookup IDs use INT PRIMARY KEY NOT NULL. In SQLite, INT PRIMARY KEY does
// not alias the rowid and does not imply NOT NULL. INTEGER PRIMARY KEY does
// alias the rowid and allocates an ID when NULL is inserted; retain that type
// only for keys that rely on this allocation, rather than fixed lookup IDs.
//
// There are parts of the schema that should never be updated in a patch/build
// release. The provider tracker requires that the underlying schema is never
// changed. Changing the schema will lead to undefined behavior and data loss.
package schema
