// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	apimetadata "github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/provisioner"
)

type metadataSuite struct {
	coretesting.FakeJujuHomeSuite
}

var _ = gc.Suite(&metadataSuite{})

func (s *metadataSuite) TestMetadataNone(c *gc.C) {
	env, ic, _ := s.setupSimpleStreamData(c, nil)

	metadata, info, err := provisioner.FindImageMetadata(env, ic, false)
	c.Assert(err, gc.ErrorMatches, ".*not found.*")
	c.Assert(info, gc.IsNil)
	c.Assert(metadata, gc.HasLen, 0)
}

func (s *metadataSuite) TestMetadataNotInStateButInDataSources(c *gc.C) {
	arch := "ppc64el"
	env, ic, setupInfo := s.setupSimpleStreamData(c, []string{arch})

	metadata, info, err := provisioner.FindImageMetadata(env, ic, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.DeepEquals, setupInfo)
	c.Assert(metadata, gc.DeepEquals, []*imagemetadata.ImageMetadata{&imagemetadata.ImageMetadata{
		Id:         "image-id",
		Arch:       arch,
		RegionName: "Region",
		Version:    "12.04",
		Endpoint:   "https://endpoint/",
	}})
}

var stateResolveInfo = &simplestreams.ResolveInfo{Source: "state server"}

func (s *metadataSuite) TestMetadataFromState(c *gc.C) {
	env, ic, _ := s.setupSimpleStreamData(c, nil)

	stored := params.CloudImageMetadata{
		ImageId: "image_id",
		Region:  "region",
		Series:  "trusty",
		Arch:    "ppc64el",
	}
	s.patchMetadataAPI(c, "", stored)

	metadata, info, err := provisioner.FindImageMetadata(env, ic, false)
	c.Assert(err, jc.ErrorIsNil)

	// This should have pulled image metadata from state server
	c.Assert(info, gc.DeepEquals, stateResolveInfo)
	c.Assert(metadata, gc.DeepEquals, []*imagemetadata.ImageMetadata{convertMetadataFromParams(stored)})
}

func (s *metadataSuite) TestMetadataStateError(c *gc.C) {
	arch := "amd64"
	env, ic, setupInfo := s.setupSimpleStreamData(c, []string{arch})

	msg := "fail"
	s.patchMetadataAPI(c, msg)

	metadata, info, err := provisioner.FindImageMetadata(env, ic, false)
	// should have logged it and proceeded to get metadata from prev search path
	// so not expecting any odd behaviour
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.DeepEquals, setupInfo)
	c.Assert(metadata, gc.DeepEquals, []*imagemetadata.ImageMetadata{&imagemetadata.ImageMetadata{
		Id:         "image-id",
		Arch:       arch,
		RegionName: "Region",
		Version:    "12.04",
		Endpoint:   "https://endpoint/",
	}})
}

func (s *metadataSuite) patchMetadataAPI(c *gc.C, errMsg string, m ...params.CloudImageMetadata) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			if errMsg != "" {
				return errors.New(errMsg)
			}
			if results, k := result.(*params.ListCloudImageMetadataResult); k {
				results.Result = append(results.Result, m...)
			}
			return nil
		})
	mockAPI := apimetadata.NewClient(apiCaller)

	s.PatchValue(provisioner.MetadataAPI, func(env environs.Environ) (*apimetadata.Client, error) {
		return mockAPI, nil
	})
	s.AddCleanup(func(*gc.C) {
		mockAPI.Close()
	})
}

func (s *metadataSuite) setupSimpleStreamData(c *gc.C, arches []string) (environs.Environ, *imagemetadata.ImageConstraint, *simplestreams.ResolveInfo) {
	dsId := "metadataSuite"
	env, cloudSpec, fileUrl := setupMetadataWithDataSource(c, s.FakeJujuHomeSuite, dsId, arches)
	ic := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
	})

	info := &simplestreams.ResolveInfo{
		Source:   dsId,
		IndexURL: fmt.Sprintf("%v%v", fileUrl, "/streams/v1/index.json"),
	}
	return env, ic, info
}

func convertMetadataFromParams(p params.CloudImageMetadata) *imagemetadata.ImageMetadata {
	m := &imagemetadata.ImageMetadata{
		Id:         p.ImageId,
		Arch:       p.Arch,
		RegionName: p.Region,
	}
	m.Version, _ = series.SeriesVersion(p.Series)
	return m
}
