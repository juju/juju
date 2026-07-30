// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"fmt"
	"slices"

	"github.com/juju/juju/core/semversion"
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
// canonical "4.0.6"-style string in both JSON and YAML.
var ExportVersions = parseExportVersions(exportVersionStrings)

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
		return fmt.Errorf("expected at least 1 entry")
	}
	for i := 1; i < len(versions); i++ {
		prev, cur := versions[i-1], versions[i]
		if cur.Compare(prev) <= 0 {
			return fmt.Errorf("entries must be strictly ascending: %s then %s", prev, cur)
		}
		if prev.Major == cur.Major && prev.Minor == cur.Minor {
			return fmt.Errorf("entries %s and %s are on the same minor line; keep only the latest", prev, cur)
		}
		nextMinor := prev.Major == cur.Major && cur.Minor == prev.Minor+1
		nextMajor := cur.Major == prev.Major+1 && cur.Minor == 0
		if !nextMinor && !nextMajor {
			return fmt.Errorf("entries %s and %s are not adjacent minor lines", prev, cur)
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
