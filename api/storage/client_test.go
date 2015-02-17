// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/storage"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
)

type storageMockSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageMockSuite{})

func (s *storageMockSuite) TestShowMix(c *gc.C) {
	var called bool

	one := "shared-fs/0"
	oneTag := names.NewStorageTag(one)
	two := "db-dir/1000"
	twoTag := names.NewStorageTag(two)
	expected := set.NewStrings(oneTag.String(), twoTag.String())

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Show")

			args, ok := a.(params.Entities)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Entities, gc.HasLen, 2)

			if results, k := result.(*params.StorageShowResults); k {
				instances := []params.StorageShowResult{
					params.StorageShowResult{
						Instance: params.StorageInstance{StorageTag: oneTag.String()},
					},
					params.StorageShowResult{
						Attachments: []params.StorageAttachment{
							params.StorageAttachment{StorageTag: twoTag.String()},
						},
					},
				}
				results.Results = instances
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	tags := []names.StorageTag{oneTag, twoTag}
	attachments, instances, err := storageClient.Show(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 1)
	c.Assert(expected.Contains(attachments[0].StorageTag), jc.IsTrue)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(expected.Contains(instances[0].StorageTag), jc.IsTrue)
	c.Assert(called, jc.IsTrue)
}

func (s *storageMockSuite) TestShowOnlyAttachments(c *gc.C) {
	var called bool

	one := "shared-fs/0"
	oneTag := names.NewStorageTag(one)
	two := "db-dir/1000"
	twoTag := names.NewStorageTag(two)
	expected := set.NewStrings(oneTag.String(), twoTag.String())

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Show")

			args, ok := a.(params.Entities)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Entities, gc.HasLen, 2)

			if results, k := result.(*params.StorageShowResults); k {
				instances := make([]params.StorageShowResult, len(args.Entities))
				for i, entity := range args.Entities {
					c.Assert(expected.Contains(entity.Tag), jc.IsTrue)
					instances[i] = params.StorageShowResult{
						Attachments: []params.StorageAttachment{
							params.StorageAttachment{StorageTag: entity.Tag},
						},
					}
				}
				results.Results = instances
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	tags := []names.StorageTag{oneTag, twoTag}
	attachments, instances, err := storageClient.Show(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 2)
	c.Assert(expected.Contains(attachments[0].StorageTag), jc.IsTrue)
	c.Assert(expected.Contains(attachments[1].StorageTag), jc.IsTrue)
	c.Assert(instances, gc.HasLen, 0)
	c.Assert(called, jc.IsTrue)
}

func (s *storageMockSuite) TestShowOnlyInstances(c *gc.C) {
	var called bool

	one := "shared-fs/0"
	oneTag := names.NewStorageTag(one)
	two := "db-dir/1000"
	twoTag := names.NewStorageTag(two)
	expected := set.NewStrings(oneTag.String(), twoTag.String())

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Show")

			args, ok := a.(params.Entities)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Entities, gc.HasLen, 2)

			if results, k := result.(*params.StorageShowResults); k {
				instances := make([]params.StorageShowResult, len(args.Entities))
				for i, entity := range args.Entities {
					c.Assert(expected.Contains(entity.Tag), jc.IsTrue)
					instances[i] = params.StorageShowResult{
						Instance: params.StorageInstance{StorageTag: entity.Tag},
					}
				}
				results.Results = instances
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	tags := []names.StorageTag{oneTag, twoTag}
	attachments, instances, err := storageClient.Show(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 0)
	c.Assert(instances, gc.HasLen, 2)
	c.Assert(expected.Contains(instances[0].StorageTag), jc.IsTrue)
	c.Assert(expected.Contains(instances[1].StorageTag), jc.IsTrue)
	c.Assert(called, jc.IsTrue)
}

func (s *storageMockSuite) TestShowFacadeCallError(c *gc.C) {
	var called bool
	one := "shared-fs/0"
	oneTag := names.NewStorageTag(one)

	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Show")

			return errors.New(msg)
		})
	storageClient := storage.NewClient(apiCaller)
	attachments, instances, err := storageClient.Show([]names.StorageTag{oneTag})
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(attachments, gc.HasLen, 0)
	c.Assert(instances, gc.HasLen, 0)
	c.Assert(called, jc.IsTrue)
}

func (s *storageMockSuite) TestShowCallError(c *gc.C) {
	var called bool
	one := "shared-fs/0"
	oneTag := names.NewStorageTag(one)

	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Show")

			if results, k := result.(*params.StorageShowResults); k {
				instances := []params.StorageShowResult{
					params.StorageShowResult{Error: common.ServerError(errors.New(msg))},
				}
				results.Results = instances
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	attachments, instances, err := storageClient.Show([]names.StorageTag{oneTag})
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(attachments, gc.HasLen, 0)
	c.Assert(instances, gc.HasLen, 0)
	c.Assert(called, jc.IsTrue)
}

func (s *storageMockSuite) TestList(c *gc.C) {
	var called bool

	one := "shared-fs/0"
	oneTag := names.NewStorageTag(one)
	two := "db-dir/1000"
	twoTag := names.NewStorageTag(two)

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "List")
			c.Check(a, gc.IsNil)

			if results, k := result.(*params.StorageListResult); k {
				results.Instances = []params.StorageInstance{
					params.StorageInstance{StorageTag: oneTag.String()}}
				results.Attachments = []params.StorageAttachment{
					params.StorageAttachment{StorageTag: twoTag.String()}}
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	attachments, instances, err := storageClient.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0].StorageTag, gc.DeepEquals, oneTag.String())
	c.Assert(attachments, gc.HasLen, 1)
	c.Assert(attachments[0].StorageTag, gc.DeepEquals, twoTag.String())
	c.Assert(called, jc.IsTrue)
}

func (s *storageMockSuite) TestListFacadeCallError(c *gc.C) {
	var called bool

	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "List")

			return errors.New(msg)
		})
	storageClient := storage.NewClient(apiCaller)
	attachments, instances, err := storageClient.List()
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(attachments, gc.HasLen, 0)
	c.Assert(instances, gc.HasLen, 0)
	c.Assert(called, jc.IsTrue)
}
