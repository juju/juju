// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package latest aliases the most recent model-export payload schema version.
//
// Code that always operates on the target (post-transform) payload — the v8
// import coordinator and the per-domain v2 import operations — references
// [ModelExport] rather than a specific versioned type. A payload-version bump
// is then a single edit here (plus adding the new transformer step), not a
// churn across every importer; importers that read only added fields keep
// compiling, and one whose field shape changed fails to compile exactly where
// it must be updated.
package latest

import (
	v4_1_0 "github.com/juju/juju/domain/export/types/v4_1_0"
)

// ModelExport is the current target model-export payload type. It tracks the
// last entry of [github.com/juju/juju/domain/export.ExportVersions] / the
// version returned by export.LatestSupportedPayloadVersion.
type ModelExport = v4_1_0.ModelExport
