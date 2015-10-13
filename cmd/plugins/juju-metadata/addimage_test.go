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

type addImageSuite struct {
	BaseClouImageMetadataSuite

	data []params.CloudImageMetadata

	mockAPI *mockAddAPI
}

var _ = gc.Suite(&addImageSuite{})

var emptyMetadata = []params.CloudImageMetadata{}

func (s *addImageSuite) SetUpTest(c *gc.C) {
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

func (s *addImageSuite) TestAddImageMetadata(c *gc.C) {
	s.assertValidAddImageMetadata(c, constructTestImageMetadata())
}

func (s *addImageSuite) TestAddImageMetadataWithStream(c *gc.C) {
	m := constructTestImageMetadata()
	m.Stream = "streamV"
	s.assertValidAddImageMetadata(c, m)
}

func (s *addImageSuite) TestAddImageMetadataWithRegion(c *gc.C) {
	m := constructTestImageMetadata()
	m.Region = "region"
	s.assertValidAddImageMetadata(c, m)
}

func (s *addImageSuite) TestAddImageMetadataAWS(c *gc.C) {
	m := constructTestImageMetadata()
	m.VirtType = "vType"
	m.RootStorageType = "sType"
	s.assertValidAddImageMetadata(c, m)
}

func (s *addImageSuite) TestAddImageMetadataAWSWithSize(c *gc.C) {
	m := constructTestImageMetadata()
	m.VirtType = "vType"
	m.RootStorageType = "sType"

	size := uint64(100)
	m.RootStorageSize = &size

	s.assertValidAddImageMetadata(c, m)
}

func (s *addImageSuite) TestAddImageMetadataFailed(c *gc.C) {
	msg := "failed"
	s.mockAPI.add = func(metadata []params.CloudImageMetadata) ([]params.ErrorResult, error) {
		return nil, errors.New(msg)
	}

	s.assertAddImageMetadataErr(c, constructTestImageMetadata(), msg)
}

func (s *addImageSuite) TestAddImageMetadataError(c *gc.C) {
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

func (s *addImageSuite) TestAddImageMetadataNoImageId(c *gc.C) {
	m := constructTestImageMetadata()
	m.ImageId = ""

	s.assertAddImageMetadataErr(c, m, "image id must be supplied when adding an image metadata")
}

func (s *addImageSuite) TestAddImageMetadataNoSeries(c *gc.C) {
	m := constructTestImageMetadata()
	m.Series = ""

	s.assertValidAddImageMetadata(c, m)
}

func (s *addImageSuite) TestAddImageMetadataNoArch(c *gc.C) {
	m := constructTestImageMetadata()
	m.Arch = ""

	s.assertValidAddImageMetadata(c, m)
}

func (s *addImageSuite) assertValidAddImageMetadata(c *gc.C, m params.CloudImageMetadata) {
	args := getAddImageMetadataCmdFlags(m)
	_, err := runAddImageMetadata(c, args...)
	c.Assert(err, jc.ErrorIsNil)

	// Need to make sure that defaults are populated
	if m.Series == "" {
		m.Series = "trusty"
	}
	if m.Arch == "" {
		m.Arch = "amd64"
	}
	if m.Stream == "" {
		m.Stream = "released"
	}

	c.Assert(s.data, gc.DeepEquals, []params.CloudImageMetadata{m})
}

func runAddImageMetadata(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&AddImageMetadataCommand{}), args...)
}

func (s *addImageSuite) assertAddImageMetadataErr(c *gc.C, m params.CloudImageMetadata, msg string) {
	args := getAddImageMetadataCmdFlags(m)
	_, err := runAddImageMetadata(c, args...)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	c.Assert(s.data, gc.DeepEquals, emptyMetadata)
}

func constructTestImageMetadata() params.CloudImageMetadata {
	return params.CloudImageMetadata{
		ImageId: "im-33333",
		Series:  "series",
		Arch:    "arch",
		Source:  "custom",
	}
}

func getAddImageMetadataCmdFlags(data params.CloudImageMetadata) []string {
	args := []string{}

	addFlag := func(flag, value, defaultValue string) {
		if value != "" {
			args = append(args, flag, value)
		} else {
			args = append(args, flag, defaultValue)
		}
	}

	addFlag("--image-id", data.ImageId, "")
	addFlag("--region", data.Region, "")
	addFlag("--series", data.Series, "trusty")
	addFlag("--arch", data.Arch, "amd64")
	addFlag("--virt-type", data.VirtType, "")
	addFlag("--storage-type", data.RootStorageType, "")
	addFlag("--stream", data.Stream, "released")

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
