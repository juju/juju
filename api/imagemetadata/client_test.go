// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/apiserver/params"
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
	series := "series"
	arch := "arch"
	virtualType := "virtual-type"
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
			c.Check(objType, gc.Equals, "ImageMetadata")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "List")

			args, ok := a.(params.ImageMetadataFilter)
			c.Assert(ok, jc.IsTrue)

			if results, k := result.(*params.ListCloudImageMetadataResult); k {
				instances := []params.CloudImageMetadata{
					params.CloudImageMetadata{
						ImageId:         imageId,
						Stream:          args.Stream,
						Region:          args.Region,
						Series:          args.Series[0],
						Arch:            args.Arches[0],
						VirtualType:     args.VirtualType,
						RootStorageType: args.RootStorageType,
						RootStorageSize: &rootStorageSize,
						Source:          source,
					},
				}
				results.Result = instances
			}

			return nil
		})
	client := imagemetadata.NewClient(apiCaller)
	found, err := client.List(
		stream, region,
		[]string{series}, []string{arch},
		virtualType, rootStorageType,
	)
	c.Check(err, jc.ErrorIsNil)

	c.Assert(called, jc.IsTrue)
	expected := []params.CloudImageMetadata{
		params.CloudImageMetadata{
			ImageId:         imageId,
			Stream:          stream,
			Region:          region,
			Series:          series,
			Arch:            arch,
			VirtualType:     virtualType,
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
			c.Check(objType, gc.Equals, "ImageMetadata")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "List")

			return errors.New(msg)
		})
	client := imagemetadata.NewClient(apiCaller)
	found, err := client.List("", "", nil, nil, "", "")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
	c.Assert(called, jc.IsTrue)
}

func (s *imagemetadataSuite) TestSave(c *gc.C) {
	m := params.CloudImageMetadata{}
	called := false

	msg := "save failure"
	expected := []params.ErrorResult{
		params.ErrorResult{},
		params.ErrorResult{&params.Error{Message: msg}},
	}

	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "ImageMetadata")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Save")

			args, ok := a.(params.MetadataSaveParams)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Metadata, gc.HasLen, 2)
			c.Assert(args.Metadata, gc.DeepEquals, []params.CloudImageMetadata{m, m})

			if results, k := result.(*params.ErrorResults); k {
				results.Results = expected
			}

			return nil
		})

	client := imagemetadata.NewClient(apiCaller)
	errs, err := client.Save([]params.CloudImageMetadata{m, m})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(errs, jc.DeepEquals, expected)
}

func (s *imagemetadataSuite) TestSaveFacadeCallError(c *gc.C) {
	m := []params.CloudImageMetadata{{}}
	called := false
	msg := "facade failure"
	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "ImageMetadata")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Save")
			return errors.New(msg)
		})
	client := imagemetadata.NewClient(apiCaller)
	found, err := client.Save(m)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
	c.Assert(called, jc.IsTrue)
}
