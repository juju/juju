// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	txntesting "github.com/juju/txn/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/state/testing"
	jujutxn "github.com/juju/txn"
)

type cloudImageMetadataSuite struct {
	testing.StateSuite
}

var _ = gc.Suite(&cloudImageMetadataSuite{})

func (s *cloudImageMetadataSuite) TestSaveMetadata(c *gc.C) {
	s.assertSaveMetadataWithDefaults(c, "stream", "series", "arch")
}

func (s *cloudImageMetadataSuite) TestSaveMetadataUpdates(c *gc.C) {
	s.assertSaveMetadataWithDefaults(c, "stream", "series", "arch")
	s.assertSaveMetadata(c, "stream", "region-test", "series",
		"arch", "virtual-type-test",
		"root-storage-type-test", "root-storage-size-test")
}

func (s *cloudImageMetadataSuite) assertSaveMetadataWithDefaults(c *gc.C, stream, series, arch string) {
	s.assertSaveMetadata(c, stream, "region", series, arch, "virtType", "rootType", "rootSize")
}

func (s *cloudImageMetadataSuite) assertSaveMetadata(c *gc.C, stream, region, series, arch, virtType, rootStorageType, rootStorageSize string) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          stream,
		Region:          region,
		Series:          series,
		Arch:            arch,
		VirtualType:     virtType,
		RootStorageType: rootStorageType,
		RootStorageSize: rootStorageSize}

	added := cloudimagemetadata.Metadata{attrs, "1"}
	s.assertRecordMetadata(c, added)
	s.assertMetadataRecorded(c, attrs, added)
}

func (s *cloudImageMetadataSuite) TestAllMetadata(c *gc.C) {
	metadata, err := s.State.Storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 0)

	m := cloudimagemetadata.Metadata{
		cloudimagemetadata.MetadataAttributes{
			Stream:          "stream",
			Region:          "region",
			Series:          "series",
			Arch:            "arch",
			VirtualType:     "virtualType",
			RootStorageType: "rootStorageType",
			RootStorageSize: "rootStorageSize"},
		"1",
	}

	s.assertRecordMetadata(c, m)

	metadata, err = s.State.Storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 1)
	expected := []cloudimagemetadata.Metadata{m}
	c.Assert(metadata, jc.SameContents, expected)

	m.Series = "series2"
	s.assertRecordMetadata(c, m)

	metadata, err = s.State.Storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 2)
	expected = append(expected, m)
	c.Assert(metadata, jc.SameContents, expected)
}

func (s *cloudImageMetadataSuite) TestFindMetadata(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region",
		Series:          "series",
		Arch:            "arch",
		VirtualType:     "virtualType",
		RootStorageType: "rootStorageType",
		RootStorageSize: "rootStorageSize"}

	m := cloudimagemetadata.Metadata{attrs, "1"}

	_, err := s.State.Storage.FindMetadata(attrs)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.assertRecordMetadata(c, m)
	expected := []cloudimagemetadata.Metadata{m}
	s.assertMetadataRecorded(c, attrs, expected...)

	attrs.Stream = "another_stream"
	m = cloudimagemetadata.Metadata{attrs, "2"}
	s.assertRecordMetadata(c, m)

	expected = append(expected, m)
	// Should find both
	s.assertMetadataRecorded(c, cloudimagemetadata.MetadataAttributes{Region: "region"}, expected...)
}

func (s *cloudImageMetadataSuite) TestSaveMetadataNoUpdates(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Series: "series",
		Arch:   "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, "1"}
	metadata1 := cloudimagemetadata.Metadata{attrs, "1"}

	s.assertRecordMetadata(c, metadata0)

	err := s.State.Storage.SaveMetadata(metadata1)
	c.Assert(err, gc.ErrorMatches, ".*no changes were made.*")

	s.assertMetadataRecorded(c, attrs, metadata0)
}

func (s *cloudImageMetadataSuite) TestSaveMetadataConcurrent(c *gc.C) {
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: s.StateSuite.MgoSuite.Session.DB("juju")})
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Series: "series",
		Arch:   "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, "0"}
	metadata1 := cloudimagemetadata.Metadata{attrs, "1"}
	metadata1.Stream = "scream"
	addMetadata := func() {
		s.assertRecordMetadata(c, metadata0)
	}

	defer txntesting.SetBeforeHooks(c, runner, addMetadata).Check()

	s.assertRecordMetadata(c, metadata1)
	s.assertMetadataRecorded(c, attrs, metadata1)
}

func (s *cloudImageMetadataSuite) assertRecordMetadata(c *gc.C, m cloudimagemetadata.Metadata) {
	err := s.State.Storage.SaveMetadata(m)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cloudImageMetadataSuite) assertMetadataRecorded(c *gc.C, criteria cloudimagemetadata.MetadataAttributes, expected ...cloudimagemetadata.Metadata) {
	metadata, err := s.State.Storage.FindMetadata(criteria)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(metadata, jc.SameContents, expected)
}
