// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/testing"
)

type AddImageSuite struct {
	BaseClouImageMetadataSuite

	data []params.CloudImageMetadata

	mockAPI *mockAddAPI
}

var _ = gc.Suite(&AddImageSuite{})

var emptyMetadata = []params.CloudImageMetadata{}

func (s *AddImageSuite) SetUpTest(c *gc.C) {
	s.BaseClouImageMetadataSuite.SetUpTest(c)

	s.data = emptyMetadata

	s.mockAPI = &mockAddAPI{}
	s.mockAPI.add = func(metadata []params.CloudImageMetadata) ([]params.ErrorResult, error) {
		s.data = append(s.data, metadata...)
		return make([]params.ErrorResult, len(metadata)), nil
	}
	s.PatchValue(&getImageMetadataAddAPI, func(c *AddImageMetadataCommand) (MetadataAddAPI, error) {
		return s.mockAPI, nil
	})
}

func (s *AddImageSuite) TestAddImageMetadata(c *gc.C) {
	s.assertValidAddImageMetadata(c, "", "", constructTestImageMetadata())
}

func (s *AddImageSuite) TestAddImageMetadataWithStream(c *gc.C) {
	m := constructTestImageMetadata()
	m.Stream = "streamV"
	s.assertValidAddImageMetadata(c, "", "", m)
}

func (s *AddImageSuite) TestAddImageMetadataAWS(c *gc.C) {
	m := constructTestImageMetadata()
	m.VirtType = "vType"
	m.RootStorageType = "sType"
	s.assertValidAddImageMetadata(c, "", "", m)
}

func (s *AddImageSuite) TestAddImageMetadataAWSWithSize(c *gc.C) {
	m := constructTestImageMetadata()
	m.VirtType = "vType"
	m.RootStorageType = "sType"

	size := uint64(100)
	m.RootStorageSize = &size

	s.assertValidAddImageMetadata(c, "", "", m)
}

func (s *AddImageSuite) TestAddImageMetadataFailed(c *gc.C) {
	msg := "failed"
	s.mockAPI.add = func(metadata []params.CloudImageMetadata) ([]params.ErrorResult, error) {
		return nil, errors.New(msg)
	}

	s.assertAddImageMetadataErr(c, constructTestImageMetadata(), msg)
}

func (s *AddImageSuite) TestAddImageMetadataError(c *gc.C) {
	msg := "failed"

	s.mockAPI.add = func(metadata []params.CloudImageMetadata) ([]params.ErrorResult, error) {
		errs := make([]params.ErrorResult, len(metadata))
		for i, _ := range metadata {
			errs[i] = params.ErrorResult{Error: common.ServerError(errors.New(msg))}
		}
		return errs, nil
	}

	s.assertAddImageMetadataErr(c, constructTestImageMetadata(), msg)
}

func (s *AddImageSuite) TestAddImageMetadataNoImageId(c *gc.C) {
	m := constructTestImageMetadata()
	m.ImageId = ""

	s.assertAddImageMetadataErr(c, m, "image id must be supplied when adding an image metadata")
}

func (s *AddImageSuite) TestAddImageMetadataNoRegion(c *gc.C) {
	m := constructTestImageMetadata()
	m.Region = ""

	s.assertAddImageMetadataErr(c, m, "region must be supplied when adding an image metadata")
}

func (s *AddImageSuite) TestAddImageMetadataNoSeries(c *gc.C) {
	m := constructTestImageMetadata()
	m.Series = ""

	s.assertAddImageMetadataErr(c, m, "series must be supplied when adding an image metadata")
}

func (s *AddImageSuite) TestAddImageMetadataNoArch(c *gc.C) {
	m := constructTestImageMetadata()
	m.Arch = ""

	s.assertAddImageMetadataErr(c, m, "architecture must be supplied when adding an image metadata")
}

func (s *AddImageSuite) assertValidAddImageMetadata(c *gc.C, expectedValid, expectedErr string, m params.CloudImageMetadata) {
	args := getAddImageMetadataCmdFlags(m)
	context, err := runAddImageMetadata(c, args...)
	c.Assert(err, jc.ErrorIsNil)

	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Matches, expectedErr)

	obtainedValid := testing.Stdout(context)
	c.Assert(obtainedValid, gc.Matches, expectedValid)

	c.Assert(s.data, gc.DeepEquals, []params.CloudImageMetadata{m})
}

func runAddImageMetadata(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&AddImageMetadataCommand{}), args...)
}

func (s *AddImageSuite) assertAddImageMetadataErr(c *gc.C, m params.CloudImageMetadata, msg string) {
	args := getAddImageMetadataCmdFlags(m)
	_, err := runAddImageMetadata(c, args...)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	c.Assert(s.data, gc.DeepEquals, emptyMetadata)
}

func constructTestImageMetadata() params.CloudImageMetadata {
	return params.CloudImageMetadata{
		ImageId: "im-33333",
		Region:  "region",
		Series:  "series",
		Arch:    "arch",
		Source:  "custom",
	}
}

func getAddImageMetadataCmdFlags(data params.CloudImageMetadata) []string {
	args := []string{}

	addFlag := func(flag, value string) {
		if value != "" {
			args = append(args, flag, value)
		}
	}

	addFlag("--image-id", data.ImageId)
	addFlag("--region", data.Region)
	addFlag("--series", data.Series)
	addFlag("--arch", data.Arch)
	addFlag("--virt-type", data.VirtType)
	addFlag("--storage-type", data.RootStorageType)
	addFlag("--stream", data.Stream)

	if data.RootStorageSize != nil {
		args = append(args, "--storage-size", fmt.Sprintf("%d", *data.RootStorageSize))
	}
	return args
}

type mockAddAPI struct {
	add func(metadata []params.CloudImageMetadata) ([]params.ErrorResult, error)
}

func (s mockAddAPI) Close() error {
	return nil
}

func (s mockAddAPI) Save(metadata []params.CloudImageMetadata) ([]params.ErrorResult, error) {
	return s.add(metadata)
}
