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

// TestExportVersionsAscending verifies the versions are listed in ascending
// order, which LatestSupportedPayloadVersion relies on.
func (s *versionSuite) TestExportVersionsAscending(c *tc.C) {
	for i := 1; i < len(ExportVersions); i++ {
		c.Check(ExportVersions[i-1].Compare(ExportVersions[i]) < 0, tc.IsTrue)
	}
}

// TestLatestSupportedPayloadVersion verifies the accessor returns the highest
// supported export schema version.
func (s *versionSuite) TestLatestSupportedPayloadVersion(c *tc.C) {
	c.Assert(LatestSupportedPayloadVersion(), tc.Equals, ExportVersions[len(ExportVersions)-1])
}
