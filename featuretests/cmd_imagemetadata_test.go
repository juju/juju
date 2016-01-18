// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	cmdmetadata "github.com/juju/juju/cmd/plugins/juju-metadata"
	"github.com/juju/juju/feature"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/testing"
)

type CmdImageMetadataSuite struct {
	jujutesting.RepoSuite
}

func (s *CmdImageMetadataSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.ImageMetadata)
}

func (s *CmdImageMetadataSuite) run(c *gc.C, args ...string) *cmd.Context {
	command := cmdmetadata.NewSuperCommand()
	context, err := testing.RunCommand(c, command, args...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *CmdImageMetadataSuite) TestAddImageCmdStack(c *gc.C) {
	s.assertNoImageMetadata(c)

	s.run(c, "add-image",
		"im-33333",
		"--series", "trusty",
		"--arch", "arch",
		"--stream", "released",
	)

	after, err := s.State.CloudImageMetadataStorage.FindMetadata(cloudimagemetadata.MetadataFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(after, gc.DeepEquals, map[string][]cloudimagemetadata.Metadata{
		"custom": []cloudimagemetadata.Metadata{{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Stream:  "released",
				Version: "14.04",
				Series:  "trusty",
				Arch:    "arch",
				Source:  "custom"},
			ImageId: "im-33333"}},
	})
}

func (s *CmdImageMetadataSuite) TestListImageCmdOk(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region",
		Version:         "14.04",
		Series:          "trusty",
		Arch:            "arch",
		VirtType:        "virtType",
		Source:          "source",
		RootStorageType: "rootStorageType"}
	m := cloudimagemetadata.Metadata{attrs, 0, "1"}
	err := s.State.CloudImageMetadataStorage.SaveMetadata([]cloudimagemetadata.Metadata{m})
	c.Assert(err, jc.ErrorIsNil)

	context := s.run(c, "list-images")

	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Matches, "")

	obtainedValid := testing.Stdout(context)
	c.Assert(obtainedValid, gc.Matches, `
SOURCE  SERIES  ARCH  REGION  IMAGE-ID  STREAM  VIRT-TYPE  STORAGE-TYPE
source  trusty  arch  region  1         stream  virtType   rootStorageType

`[1:])
}

func (s *CmdImageMetadataSuite) TestDeleteImageCmdOk(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region",
		Version:         "14.04",
		Series:          "trusty",
		Arch:            "arch",
		VirtType:        "virtType",
		Source:          "source",
		RootStorageType: "rootStorageType"}
	m := cloudimagemetadata.Metadata{attrs, 0, "1"}
	err := s.State.CloudImageMetadataStorage.SaveMetadata([]cloudimagemetadata.Metadata{m})
	c.Assert(err, jc.ErrorIsNil)

	context := s.run(c, "delete-image", "1")

	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Matches, "")

	obtainedValid := testing.Stdout(context)
	c.Assert(obtainedValid, gc.Matches, "")

	s.assertNoImageMetadata(c)
}

func (s *CmdImageMetadataSuite) assertNoImageMetadata(c *gc.C) {
	before, err := s.State.CloudImageMetadataStorage.FindMetadata(cloudimagemetadata.MetadataFilter{})
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, "matching cloud image metadata not found")
	c.Assert(before, gc.HasLen, 0)
}
