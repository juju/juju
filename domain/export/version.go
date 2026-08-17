// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"fmt"
	"slices"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/errors"
)

// exportVersionStrings lists, in ascending order, each semantic version for
// which there is a new export format. This is the editable source of truth: to
// generate new export types and logic, add the current semantic version in
// string form, then run `go generate` from the generate/export directory. If
// the version currently being worked on has not been released, the generation
// can be run repeatedly for the same version.
var exportVersionStrings = []string{
	"4.0.12",
	"4.1.0", // mutable, not released
}

// ExportVersions lists each semantic version for which there is a new export
// format, in ascending order. It is derived from [exportVersionStrings]; the
// wire format is unchanged because [semversion.Number] marshals to the
// canonical "4.0.12"-style string in both JSON and YAML.
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

// OldestSupportedPayloadVersion returns the lowest supported model-export
// payload schema version: the floor of this controller's import window. A
// source stamping a payload below this floor cannot be imported directly and
// has to come up through the intervening releases first.
func OldestSupportedPayloadVersion() semversion.Number {
	return slices.MinFunc(ExportVersions, semversion.Number.Compare)
}

// CheckPayloadVersionSupported reports whether this controller can import a
// model export payload stamped with the given schema version, returning an
// error satisfying [coreerrors.NotSupported] when it cannot.
//
// The message names the controller the operator has to act on, because that is
// the one thing the raw version number does not tell them. A source only ever
// stamps the payload with the export format compiled into its own binary, so a
// version this controller does not hold means one side is behind the other:
// either the source is old and must be upgraded, or this controller is and the
// migration cannot proceed until the target catches up.
func CheckPayloadVersionSupported(version semversion.Number) error {
	if slices.Contains(ExportVersions, version) {
		return nil
	}

	// Ahead of everything we know: the source runs a newer Juju than this
	// controller, so the target is the side to move.
	if latest := LatestSupportedPayloadVersion(); version.Compare(latest) > 0 {
		return errors.Errorf(
			"source payload version %q is newer than target %q; upgrade the target controller first: %w",
			version, latest, coreerrors.NotSupported)
	}

	// On a minor line we do import, but at a different patch. Only one format
	// per line is supported, so the side holding the older one has to move; an
	// in-place upgrade to the latest patch of that line needs no migration.
	if entry, ok := supportedVersionForLine(version); ok {
		line := fmt.Sprintf("%d.%d", version.Major, version.Minor)
		if version.Compare(entry) > 0 {
			return errors.Errorf(
				"source payload version %q is newer than the %s export format this controller imports (%q); upgrade the target controller first: %w",
				version, line, entry, coreerrors.NotSupported)
		}
		return errors.Errorf(
			"source payload version %q predates the %s export format this controller imports (%q); upgrade the source controller in place to the latest %s release, then retry the migration: %w",
			version, line, entry, line, coreerrors.NotSupported)
	}

	// Below the floor: too old to reach this controller in a single hop.
	if oldest := OldestSupportedPayloadVersion(); version.Compare(oldest) < 0 {
		return errors.Errorf(
			"source payload version %q is older than the oldest export format this controller imports (%q); upgrade the source controller through the intervening releases first: %w",
			version, oldest, coreerrors.NotSupported)
	}

	return errors.Errorf(
		"model export payload version %q is not one of the export formats this controller imports (%v): %w",
		version, ExportVersions, coreerrors.NotSupported)
}

// supportedVersionForLine returns the supported export version sharing the
// given version's major.minor line, if this controller holds one. At most one
// entry exists per minor line: the patch at which that line's schema last
// changed.
func supportedVersionForLine(version semversion.Number) (semversion.Number, bool) {
	for _, supported := range ExportVersions {
		if supported.Major == version.Major && supported.Minor == version.Minor {
			return supported, true
		}
	}
	return semversion.Number{}, false
}
