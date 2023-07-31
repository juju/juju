// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager_test

import (
	"regexp"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/client/imagemetadatamanager"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type imagemetadataSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&imagemetadataSuite{})

func (s *imagemetadataSuite) TestList(c *gc.C) {
	// setup data for test
	imageId := "imageid"
	stream := "stream"
	region := "region"

	// This is used by filters to search function
	base := corebase.MustParseBaseFromString("ubuntu@22.04")
	version := base.Channel.Track

	arch := "arch"
	virtType := "virt-type"
	rootStorageType := "root-storage-type"
	rootStorageSize := uint64(1024)
	source := "source"

	called := false
	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "ImageMetadataManager")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "List")

			args, ok := a.(params.ImageMetadataFilter)
			c.Assert(ok, jc.IsTrue)

			if results, k := result.(*params.ListCloudImageMetadataResult); k {
				instances := []params.CloudImageMetadata{
					{
						ImageId:         imageId,
						Stream:          args.Stream,
						Region:          args.Region,
						Version:         args.Versions[0],
						Arch:            args.Arches[0],
						VirtType:        args.VirtType,
						RootStorageType: args.RootStorageType,
						RootStorageSize: &rootStorageSize,
						Source:          source,
					},
				}
				results.Result = instances
			}

			return nil
		})
	client := imagemetadatamanager.NewClient(apiCaller)
	found, err := client.List(
		stream, region,
		[]corebase.Base{base}, []string{arch},
		virtType, rootStorageType,
	)
	c.Check(err, jc.ErrorIsNil)

	c.Assert(called, jc.IsTrue)
	expected := []params.CloudImageMetadata{
		{
			ImageId:         imageId,
			Stream:          stream,
			Region:          region,
			Version:         version,
			Arch:            arch,
			VirtType:        virtType,
			RootStorageType: rootStorageType,
			RootStorageSize: &rootStorageSize,
			Source:          source,
		},
	}
	c.Assert(found, jc.DeepEquals, expected)
}

func (s *imagemetadataSuite) TestListFacadeCallError(c *gc.C) {
	msg := "facade failure"
	called := false
	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "ImageMetadataManager")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "List")

			return errors.New(msg)
		})
	client := imagemetadatamanager.NewClient(apiCaller)
	found, err := client.List("", "", nil, nil, "", "")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
	c.Assert(called, jc.IsTrue)
}

func (s *imagemetadataSuite) TestSave(c *gc.C) {
	m := params.CloudImageMetadata{}
	called := false

	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "ImageMetadataManager")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Save")

			c.Assert(a, gc.FitsTypeOf, params.MetadataSaveParams{})
			args := a.(params.MetadataSaveParams)
			c.Assert(args.Metadata, gc.HasLen, 1)
			c.Assert(args.Metadata, jc.DeepEquals, []params.CloudImageMetadataList{
				{[]params.CloudImageMetadata{m, m}},
			})

			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}

			return nil
		})

	client := imagemetadatamanager.NewClient(apiCaller)
	err := client.Save([]params.CloudImageMetadata{m, m})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *imagemetadataSuite) TestSaveFacadeCallError(c *gc.C) {
	m := []params.CloudImageMetadata{{}}
	msg := "facade failure"
	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ImageMetadataManager")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Save")
			return errors.New(msg)
		})
	client := imagemetadatamanager.NewClient(apiCaller)
	err := client.Save(m)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *imagemetadataSuite) TestSaveFacadeCallErrorResult(c *gc.C) {
	m := []params.CloudImageMetadata{{}}
	msg := "facade failure"
	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ImageMetadataManager")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Save")
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{
					{Error: &params.Error{Message: msg}},
				},
			}
			return nil
		})
	client := imagemetadatamanager.NewClient(apiCaller)
	err := client.Save(m)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *imagemetadataSuite) TestDelete(c *gc.C) {
	imageId := "tst12345"
	called := false

	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "ImageMetadataManager")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Delete")

			c.Assert(a, gc.FitsTypeOf, params.MetadataImageIds{})
			c.Assert(a.(params.MetadataImageIds).Ids, gc.DeepEquals, []string{imageId})

			results := result.(*params.ErrorResults)
			results.Results = []params.ErrorResult{{}}
			return nil
		})

	client := imagemetadatamanager.NewClient(apiCaller)
	err := client.Delete(imageId)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *imagemetadataSuite) TestDeleteMultipleResult(c *gc.C) {
	imageId := "tst12345"
	called := false

	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "ImageMetadataManager")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Delete")

			results := result.(*params.ErrorResults)
			results.Results = []params.ErrorResult{{}, {}}
			return nil
		})

	client := imagemetadatamanager.NewClient(apiCaller)
	err := client.Delete(imageId)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`expected to find one result for image id "tst12345" but found 2`))
	c.Assert(called, jc.IsTrue)
}

func (s *imagemetadataSuite) TestDeleteFailure(c *gc.C) {
	called := false
	msg := "save failure"

	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "ImageMetadataManager")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Delete")

			results := result.(*params.ErrorResults)
			results.Results = []params.ErrorResult{
				{&params.Error{Message: msg}},
			}
			return nil
		})

	client := imagemetadatamanager.NewClient(apiCaller)
	err := client.Delete("tst12345")
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}

func (s *imagemetadataSuite) TestDeleteFacadeCallError(c *gc.C) {
	called := false
	msg := "facade failure"
	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "ImageMetadataManager")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Delete")
			return errors.New(msg)
		})
	client := imagemetadatamanager.NewClient(apiCaller)
	err := client.Delete("tst12345")
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}
