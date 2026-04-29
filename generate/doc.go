// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package generate provides code generation tools for Juju.
//
// These tools are build-time programs invoked via go generate directives or
// Make targets that transform inputs into generated artifacts as below:
//
//   - certgen transforms PKI profiles into test certificates
//   - cloudcred transforms registered provider schemas into credential metadata
//   - ddlgen transforms database schemas into DDL snapshots for verification
//   - export transforms database schemas into export-related code
//   - filetoconst transforms files into Go string constants
//   - schemagen transforms API facade definitions into JSON schemas
//   - triggergen transforms database table definitions into change triggers
//
// See the subpackages for individual generator documentation. See the
// Makefile for build-time generation targets (schema-gen, trigger-gen, ddl-gen,
// export-gen). See go generate directives in domain/schema/controller.go and
// domain/schema/model.go for trigger generation patterns.
package generate
