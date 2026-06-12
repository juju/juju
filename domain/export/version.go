// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"slices"

	"github.com/juju/juju/core/semversion"
)

// exportVersionStrings lists, in ascending order, each semantic version for
// which there is a new export format. This is the editable source of truth: to
// generate new export types and logic, add the current semantic version in
// string form, then run `go generate` from the generate/export directory. If
// the version currently being worked on has not been released, the generation
// can be run repeatedly for the same version.
var exportVersionStrings = []string{
	"4.0.4",
	"4.0.6",
}

// ExportVersions lists each semantic version for which there is a new export
// format, in ascending order. It is derived from [exportVersionStrings]; the
// wire format is unchanged because [semversion.Number] marshals to the
// canonical "4.0.6"-style string in both JSON and YAML.
var ExportVersions = parseExportVersions(exportVersionStrings)

func parseExportVersions(versions []string) []semversion.Number {
	parsed := make([]semversion.Number, len(versions))
	for i, v := range versions {
		parsed[i] = semversion.MustParse(v)
	}
	return parsed
}

// LatestSupportedPayloadVersion returns the highest supported model-export
// payload schema version. This is the single authority for target-side
// "targetVersion" comparisons in the v8 migration import path (v8 Prechecks and
// ModelImporterV2); it is the model-export schema version, not the controller
// binary version, and must not be confused with GetControllerTargetVersion.
func LatestSupportedPayloadVersion() semversion.Number {
	return slices.MaxFunc(ExportVersions, semversion.Number.Compare)
}
