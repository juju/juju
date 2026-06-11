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
		semversion.MustParse("4.0.6"),
	)
}
