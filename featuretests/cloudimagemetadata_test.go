// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/state/testing"
)

type cloudImageMetadataSuite struct {
	testing.StateSuite
}

func (s *cloudImageMetadataSuite) TestSaveAndFindMetadata(c *gc.C) {
	metadata, err := s.State.CloudImageMetadataStorage.FindMetadata(cloudimagemetadata.MetadataAttributes{})
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, "matching cloud image metadata not found")
	c.Assert(metadata, gc.HasLen, 0)

	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region",
		Series:          "series",
		Arch:            "arch",
		VirtualType:     "virtType",
		RootStorageType: "rootStorageType",
		RootStorageSize: "rootStorageSize"}

	m := cloudimagemetadata.Metadata{attrs, "1"}
	err = s.State.CloudImageMetadataStorage.SaveMetadata(m)
	c.Assert(err, jc.ErrorIsNil)

	added, err := s.State.CloudImageMetadataStorage.FindMetadata(attrs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(added, jc.SameContents, []cloudimagemetadata.Metadata{m})
}
