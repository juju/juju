// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
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
	c.Check(
		LatestSupportedPayloadVersion(),
		tc.Equals,
		semversion.MustParse("4.1.0"),
	)
}

// TestOldestSupportedPayloadVersionCurrent documents the floor of the import
// window. Update this when the oldest supported export version moves.
func (s *versionSuite) TestOldestSupportedPayloadVersionCurrent(c *tc.C) {
	c.Check(
		OldestSupportedPayloadVersion(),
		tc.Equals,
		semversion.MustParse("4.0.12"),
	)
}

// TestExportVersionsNonEmpty ensures that the support-window helpers have an
// input to inspect.
func (s *versionSuite) TestExportVersionsNonEmpty(c *tc.C) {
	c.Check(len(ExportVersions) > 0, tc.IsTrue)
}

// TestExportVersionsOnePerMinorLine ensures each export format corresponds to
// exactly one schema version per major.minor release line.
func (s *versionSuite) TestExportVersionsOnePerMinorLine(c *tc.C) {
	versionsByMajorMinor := make(map[string]int)
	for _, version := range ExportVersions {
		majorMinor := fmt.Sprintf("%d.%d", version.Major, version.Minor)
		versionsByMajorMinor[majorMinor]++
	}
	for majorMinor, count := range versionsByMajorMinor {
		c.Check(count, tc.Equals, 1,
			tc.Commentf("multiple export versions for %s", majorMinor))
	}
}

// TestCheckPayloadVersionSupported verifies that every supported export
// version is accepted.
func (s *versionSuite) TestCheckPayloadVersionSupported(c *tc.C) {
	for _, version := range ExportVersions {
		c.Check(CheckPayloadVersionSupported(version), tc.ErrorIsNil,
			tc.Commentf("export version %q must be supported", version))
	}
}

// TestCheckPayloadVersionRejections verifies that each way a payload version
// can fall outside the import window names the controller the operator has to
// upgrade.
func (s *versionSuite) TestCheckPayloadVersionRejections(c *tc.C) {
	tests := []struct {
		summary string
		version string
		expect  string
	}{{
		summary: "newer than every format we hold: the target is behind",
		version: "9.9.9",
		expect:  `source payload version "9.9.9" is newer than target "4.1.0"; upgrade the target controller first.*`,
	}, {
		summary: "older patch of a line we import: the source is behind",
		version: "4.0.11",
		expect:  `source payload version "4.0.11" predates the 4.0 export format this controller imports \("4.0.12"\); upgrade the source controller in place to the latest 4.0 release, then retry the migration.*`,
	}, {
		summary: "newer patch of a line we import: the target is behind on that line",
		version: "4.0.13",
		expect:  `source payload version "4.0.13" is newer than the 4.0 export format this controller imports \("4.0.12"\); upgrade the target controller first.*`,
	}, {
		summary: "below the floor: too old to reach us in one hop",
		version: "3.6.1",
		expect:  `source payload version "3.6.1" is older than the oldest export format this controller imports \("4.0.12"\); upgrade the source controller through the intervening releases first.*`,
	}}

	for _, test := range tests {
		c.Logf("%s", test.summary)
		err := CheckPayloadVersionSupported(semversion.MustParse(test.version))
		c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
		c.Check(err, tc.ErrorMatches, test.expect)
	}
}

// TestCheckPayloadVersionSupportedSparseVersions verifies that an unsupported
// version in a gap between supported release lines receives the fallback error.
func (s *versionSuite) TestCheckPayloadVersionSupportedSparseVersions(c *tc.C) {
	originalVersions := ExportVersions
	ExportVersions = []semversion.Number{
		semversion.MustParse("4.0.12"),
		semversion.MustParse("4.2.0"),
	}
	c.Cleanup(func() {
		ExportVersions = originalVersions
	})

	err := CheckPayloadVersionSupported(semversion.MustParse("4.1.5"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
	c.Check(err, tc.ErrorMatches,
		`model export payload version "4.1.5" is not one of the export formats this controller imports \(\[4.0.12 4.2.0\]\).*`)
}
