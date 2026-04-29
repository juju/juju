// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package generate provides code generation tools for Juju.
//
// Code generators are Go programs that produce source files, schemas, and test
// data from templates, database schemas, or API definitions. Generators are
// invoked via go generate directives embedded in source files or via Make
// targets -- filetoconst converts files to Go constants, schemagen produces
// JSON schemas for API facades, ddlgen creates DDL snapshots for schema
// verification, triggergen generates database change triggers, certgen creates
// test certificates, cloudcred generates credential schemas from registered
// providers, and export generates export-related code from database schemas.
//
// See the subpackages below for individual generator documentation. See the
// Makefile for build-time generation targets (schema-gen, trigger-gen, ddl-gen,
// export-gen). See go generate directives in domain/schema/controller.go and
// domain/schema/model.go for trigger generation patterns.
package generate
