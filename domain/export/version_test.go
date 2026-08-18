// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
)

type versionSuite struct{}

func TestVersionSuite(t *testing.T) {
	tc.Run(t, &versionSuite{})
}

// TestExportVersionsParsed verifies that every editable version string parses
// to a canonical semversion.Number and that the typed slice preserves order.
func (s *versionSuite) TestExportVersionsParsed(c *tc.C) {
	c.Assert(len(ExportVersions), tc.Equals, len(exportVersionStrings))
	for i, v := range exportVersionStrings {
		c.Check(ExportVersions[i], tc.Equals, semversion.MustParse(v))
	}
}

// TestLatestSupportedPayloadVersion documents the current highest supported
// export schema version. Update this when adding a new export payload version.
func (s *versionSuite) TestLatestSupportedPayloadVersionCurrent(c *tc.C) {
	c.Assert(
		LatestSupportedPayloadVersion(),
		tc.Equals,
		semversion.MustParse("4.0.12"),
	)
}

// TestValidateExportVersions pins the exportVersionStrings contract: at
// least one entry, strictly ascending, one entry per minor line, adjacent
// minor lines throughout.
func (s *versionSuite) TestValidateExportVersions(c *tc.C) {
	parse := func(vs ...string) []semversion.Number {
		parsed := make([]semversion.Number, len(vs))
		for i, v := range vs {
			parsed[i] = semversion.MustParse(v)
		}
		return parsed
	}

	for _, valid := range [][]semversion.Number{
		parse("4.0.12"),
		parse("4.0.12", "4.1.0"),
		parse("4.0.12", "4.1.3", "4.2.0"),
		parse("4.9.9", "5.0.0"),
	} {
		c.Check(validateExportVersions(valid), tc.ErrorIsNil)
	}

	for _, test := range []struct {
		desc     string
		versions []semversion.Number
		errMatch string
	}{
		{"empty", parse(), "expected at least 1 entry"},
		{"same minor line", parse("4.0.6", "4.0.12"), "same minor line"},
		{"descending", parse("4.1.0", "4.0.12"), "strictly ascending"},
		{"duplicate", parse("4.0.12", "4.0.12"), "strictly ascending"},
		{"non-adjacent pair", parse("4.0.12", "4.2.0"), "not adjacent minor lines"},
		{"gap mid-chain", parse("4.0.12", "4.1.0", "4.3.0"), "not adjacent minor lines"},
	} {
		err := validateExportVersions(test.versions)
		c.Assert(err, tc.NotNil)
		c.Check(err.Error(), tc.Matches, ".*"+test.errMatch+".*")
	}
}
