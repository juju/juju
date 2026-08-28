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

// exportVersionStrings is the editable source of truth for the model-export
// payload schema versions this controller can produce or consume.
//
// Contract:
//   - Entries are payload *schema* versions, not release versions: one entry
//     per supported minor line, holding the patch version at which the
//     model-DB schema (and therefore the export format) last changed on
//     that line (the in-dev version until it ships). Entries move with the
//     schema; they never accumulate within a line.
//   - The list spans the supported import window: from the oldest minor
//     line this branch accepts migrations from (the floor) to its own line.
//     A source-only branch (exports but cannot import the new format) lists
//     exactly one entry: its own line.
//   - The transformer walks a payload one hop per adjacent pair, from the
//     source's line up to the own entry. Each hop is a registered
//     version-to-version transform step; imports stamped with the own entry
//     are a no-op.
//   - Imports stamped with any unlisted version are rejected at
//     precheck/import. Sources on an older patch of a listed line are
//     directed to upgrade the source controller in place to the latest
//     patch of their line first (in-place upgrades need no migration);
//     sources older than the floor line must migrate via intermediate
//     minor releases.
//   - The own entry is frozen once a release ships it: the first model-DB
//     schema change after that release must move it and regenerate; never
//     regenerate types under an already-released version. Unreleased
//     entries may be regenerated freely. Non-own entries are NOT frozen:
//     they follow their source branch as that line's latest format moves
//     (the CI sync check enforces this).
//
// To bump: move this branch's entry (or append it on a target branch),
// then run `go generate` from generate/export. On a target branch also
// vendor domain/export/types/v<prev>/ from the previous-minor branch and
// delete the superseded types directory when a non-own entry moves.
var exportVersionStrings = []string{
	"4.0.12",
	"4.1.0", // mutable, not released
}

// ExportVersions lists each semantic version for which there is a new export
// format, in ascending order. It is derived from [exportVersionStrings]; the
// wire format is unchanged because [semversion.Number] marshals to the
// canonical "4.0.12"-style string in both JSON and YAML.
var ExportVersions = parseExportVersions(exportVersionStrings)

// controllerExportVersionStrings lists, in ascending order, each semantic
// version for which there is a new controller-export format. Editable source
// of truth for the controller-export generator pass; the controller schema
// evolves independently of the model schema.
var controllerExportVersionStrings = []string{
	"4.1.0",
}

// ControllerExportVersions lists each semantic version for which there is a
// new controller-export format, in ascending order. It is derived from
// [controllerExportVersionStrings].
var ControllerExportVersions = parseExportVersions(controllerExportVersionStrings)

func parseExportVersions(versions []string) []semversion.Number {
	parsed := make([]semversion.Number, len(versions))
	for i, v := range versions {
		parsed[i] = semversion.MustParse(v)
	}
	if err := validateExportVersions(parsed); err != nil {
		panic(fmt.Sprintf("invalid exportVersionStrings: %v", err))
	}
	return parsed
}

// validateExportVersions enforces the exportVersionStrings contract: at
// least one entry, strictly ascending, never two entries from the same
// minor line, and adjacent minor lines throughout (same major, minor+1;
// or next major at minor 0).
func validateExportVersions(versions []semversion.Number) error {
	if len(versions) == 0 {
		return errors.Errorf("expected at least 1 entry")
	}
	for i := 1; i < len(versions); i++ {
		prev, cur := versions[i-1], versions[i]
		if cur.Compare(prev) <= 0 {
			return errors.Errorf("entries must be strictly ascending: %s then %s", prev, cur)
		}
		if prev.Major == cur.Major && prev.Minor == cur.Minor {
			return errors.Errorf("entries %s and %s are on the same minor line; keep only the latest", prev, cur)
		}
		nextMinor := prev.Major == cur.Major && cur.Minor == prev.Minor+1
		nextMajor := cur.Major == prev.Major+1 && cur.Minor == 0
		if !nextMinor && !nextMajor {
			return errors.Errorf("entries %s and %s are not adjacent minor lines", prev, cur)
		}
	}
	return nil
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

// LatestControllerExportVersion returns the highest supported
// controller-export schema version. Separate from the model-export versions:
// the controller schema evolves independently.
func LatestControllerExportVersion() semversion.Number {
	return slices.MaxFunc(ControllerExportVersions, semversion.Number.Compare)
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
	if supported, ok := supportedVersionForLine(version); ok {
		majorMinor := fmt.Sprintf("%d.%d", version.Major, version.Minor)
		if version.Compare(supported) > 0 {
			return errors.Errorf(
				"source payload version %q is newer than the %s export format this controller imports (%q); upgrade the target controller first: %w",
				version, majorMinor, supported, coreerrors.NotSupported)
		}
		return errors.Errorf(
			"source payload version %q predates the %s export format this controller imports (%q); upgrade the source controller in place to the latest %s release, then retry the migration: %w",
			version, majorMinor, supported, majorMinor, coreerrors.NotSupported)
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
