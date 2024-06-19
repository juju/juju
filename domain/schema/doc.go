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
// There are parts of the schema that should never be updated in a patch/build
// release. The provider tracker requires that the underlying schema is never
// changed. Changing the schema will lead to undefined behavior and data loss.
package schema
