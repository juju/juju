// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package latest aliases the most recent model-export payload schema version.
//
// Code that always operates on the target (post-transform) payload -- the v8
// import coordinator and the per-domain v2 import operations -- references
// [ModelExport] rather than a specific versioned type. A payload-version bump
// is then a single edit here (plus adding the new transformer step), not a
// churn across every importer; importers that read only added fields keep
// compiling, and one whose field shape changed fails to compile exactly where
// it must be updated.
//
// See [github.com/juju/juju/domain/export] for the version registry and
// payload projection helpers. See [github.com/juju/juju/domain/modelimport]
// for the transformation wiring that normalizes older payloads.
package latest
