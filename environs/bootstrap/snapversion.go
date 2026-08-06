// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"fmt"
	"regexp"

	"github.com/juju/juju/core/semversion"
)

// snapVersionSuffixRe matches the build-time suffix appended to a snap
// version: either an optional +<segment> immediately followed by a trailing
// -[0-9a-f]{7}, or a bare trailing -[0-9a-f]{7}.
var snapVersionSuffixRe = regexp.MustCompile(`(?:\+[^\s+-]+)?-[0-9a-f]{7}$`)

// ParseSnapVersion parses a controller snap's version string (the value
// declared in a snapcraft.yaml `version:` field and copied into
// meta/snap.yaml) into a semversion.Number.
//
// The published jujud snap follows the juju snap version convention and can
// carry a build-time suffix that semversion.Parse rejects, for example:
//
//	4.1-beta2               (plain tagged version)
//	4.1-beta2-06aa059       (edge: -<sha7>)
//	4.0.10-e0c5d0b          (edge: -<sha7>)
//	4.1-beta2+main-06aa059  (devel: +<branch>-<sha7>)
//	4.1-beta2.3             (build number, never stripped)
//
// The raw value is parsed with semversion.Parse first; if it succeeds nothing
// is stripped, so a valid raw semantic version such as `4.1-abcdef1` is
// preserved rather than mistaken for a sha suffix. Otherwise the suffix rule
// above is applied and the result is re-parsed. The raw string must be
// preserved by callers that need an exact installed-version assertion; this
// function only derives the normalised Number.
func ParseSnapVersion(raw string) (semversion.Number, error) {
	vers, err := semversion.Parse(raw)
	if err == nil {
		return vers, nil
	}

	base := snapVersionSuffixRe.ReplaceAllString(raw, "")
	if base == raw {
		return semversion.Zero, fmt.Errorf("invalid snap version %q", raw)
	}
	vers, err = semversion.Parse(base)
	if err != nil {
		return semversion.Zero, fmt.Errorf("invalid snap version %q", raw)
	}
	return vers, nil
}
