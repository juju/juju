// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

import (
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type cloudImageMetadataSuite struct{}

var _ = gc.Suite(&cloudImageMetadataSuite{})

func (s *cloudImageMetadataSuite) TestCloudImageMetadataDocFields(c *gc.C) {
	ignored := set.NewStrings("Id")
	migrated := set.NewStrings(
		"Stream",
		"Region",
		"Version",
		"Series",
		"Arch",
		"VirtType",
		"RootStorageType",
		"RootStorageSize",
		"Source",
		"Priority",
		"ImageId",
		"DateCreated",
	)
	fields := migrated.Union(ignored)
	expected := testing.GetExportedFields(imagesMetadataDoc{})
	unknown := expected.Difference(fields)
	removed := fields.Difference(expected)
	// If this test fails, it means that extra fields have been added to the
	// doc without thinking about the migration implications.
	c.Check(unknown, gc.HasLen, 0)
	c.Assert(removed, gc.HasLen, 0)
}
