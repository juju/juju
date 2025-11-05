// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package modelmigration provides the coordination framework for migrating
// models between controllers.
//
// Model migration is a complex process that requires coordinating the export
// and import of data across multiple domains. This package provides the
// registry and coordination mechanisms for domain-specific migration handlers.
//
// # Migration Process
//
// Model migration involves:
//   - Exporting model data from the source controller
//   - Validating and transforming the exported data
//   - Importing the data into the target controller
//   - Coordinating dependencies between domains
//
// Each domain can register its own export and import operations, which are
// called in dependency order during the migration process.
//
// # Domain Integration
//
// Domains participate in migration by providing modelmigration subdirectories
// with export and import implementations that handle their specific data.
package modelmigration
