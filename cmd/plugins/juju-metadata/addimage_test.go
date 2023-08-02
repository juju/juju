// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type addImageSuite struct {
	BaseCloudImageMetadataSuite

	data []params.CloudImageMetadata

	mockAPI *mockAddAPI
}

var _ = gc.Suite(&addImageSuite{})

var emptyMetadata = []params.CloudImageMetadata{}

func (s *addImageSuite) SetUpTest(c *gc.C) {
	s.BaseCloudImageMetadataSuite.SetUpTest(c)

	s.data = emptyMetadata

	s.mockAPI = &mockAddAPI{}
	s.mockAPI.add = func(metadata []params.CloudImageMetadata) error {
		s.data = append(s.data, metadata...)
		return nil
	}
	s.PatchValue(&getImageMetadataAddAPI, func(c *addImageMetadataCommand) (MetadataAddAPI, error) {
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
	s.mockAPI.add = func(metadata []params.CloudImageMetadata) error {
		return errors.New(msg)
	}

	s.assertAddImageMetadataErr(c, constructTestImageMetadata(), msg)
}

func (s *addImageSuite) TestAddImageMetadataNoImageId(c *gc.C) {
	m := constructTestImageMetadata()
	m.ImageId = ""

	s.assertAddImageMetadataErr(c, m, "image id must be supplied when adding image metadata")
}

func (s *addImageSuite) TestAddImageMetadataNoSeries(c *gc.C) {
	m := constructTestImageMetadata()
	m.Version = ""
	// OSType will default to config default, for e.g. "jammy"
	s.assertValidAddImageMetadata(c, m)
}

func (s *addImageSuite) TestAddImageMetadataNoArch(c *gc.C) {
	m := constructTestImageMetadata()
	m.Arch = ""

	s.assertValidAddImageMetadata(c, m)
}

func (s *addImageSuite) assertValidAddImageMetadata(c *gc.C, m params.CloudImageMetadata) {
	args := getAddImageMetadataCmdFlags(c, m)

	_, err := runAddImageMetadata(c, args...)
	c.Assert(err, jc.ErrorIsNil)

	// Need to make sure that defaults are populated
	if m.Arch == "" {
		m.Arch = "amd64"
	}
	if m.Stream == "" {
		m.Stream = "released"
	}

	c.Assert(s.data, gc.DeepEquals, []params.CloudImageMetadata{m})
}

func runAddImageMetadata(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := &addImageMetadataCommand{}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return cmdtesting.RunCommand(c, modelcmd.Wrap(cmd), args...)
}

func (s *addImageSuite) assertAddImageMetadataErr(c *gc.C, m params.CloudImageMetadata, msg string) {
	args := getAddImageMetadataCmdFlags(c, m)
	_, err := runAddImageMetadata(c, args...)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	c.Assert(s.data, gc.DeepEquals, emptyMetadata)
}

func constructTestImageMetadata() params.CloudImageMetadata {
	return params.CloudImageMetadata{
		ImageId:  "im-33333",
		Version:  "22.04",
		Arch:     "arch",
		Source:   "custom",
		Priority: 50,
	}
}

func getAddImageMetadataCmdFlags(c *gc.C, data params.CloudImageMetadata) []string {
	args := []string{}

	addFlag := func(flag, value, defaultValue string) {
		if value != "" {
			args = append(args, flag, value)
		} else {
			if defaultValue != "" {
				args = append(args, flag, defaultValue)
			}
		}
	}

	var aBase string
	if data.Version != "" {
		aBase = corebase.MakeDefaultBase("ubuntu", data.Version).String()
	}
	addFlag("--base", aBase, "")
	addFlag("--region", data.Region, "")
	addFlag("--arch", data.Arch, "amd64")
	addFlag("--virt-type", data.VirtType, "")
	addFlag("--storage-type", data.RootStorageType, "")
	addFlag("--stream", data.Stream, "released")

	if data.RootStorageSize != nil {
		args = append(args, "--storage-size", fmt.Sprintf("%d", *data.RootStorageSize))
	}

	// image id is an argument
	if data.ImageId != "" {
		args = append(args, data.ImageId)
	}
	return args
}

type mockAddAPI struct {
	add func(metadata []params.CloudImageMetadata) error
}

func (s mockAddAPI) Close() error {
	return nil
}

func (s mockAddAPI) Save(metadata []params.CloudImageMetadata) error {
	return s.add(metadata)
}
