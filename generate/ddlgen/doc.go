// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// ddlgen is a tool used to generate the DDLs for the model and controller
// databases for every patch version of this major.minor (with the exception of
// the current version i.e. the in development version).
//
// We use this to verify that DDLs do not change in invalid ways.
//
// We need this because, to support controller upgrades, the only changes we can make
// to the DDL between patch releases are additive. Applying the DDL is idempotent
// using stored hashes of the patches we've already applied. These hashes are
// calculated before the DDL is parsed, meaning comments, whitespace, etc. also
// cannot change.
package main
