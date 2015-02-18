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

func (s *storageMockSuite) TestShow(c *gc.C) {
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
						Result: params.StorageInfo{StorageTag: oneTag.String()},
					},
					params.StorageShowResult{
						Result: params.StorageInfo{
							StorageTag: twoTag.String(),
							Attached:   true,
						},
					},
				}
				results.Results = instances
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	tags := []names.StorageTag{oneTag, twoTag}
	found, err := storageClient.Show(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 2)
	c.Assert(expected.Contains(found[0].StorageTag), jc.IsTrue)
	c.Assert(expected.Contains(found[1].StorageTag), jc.IsTrue)
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
	found, err := storageClient.Show([]names.StorageTag{oneTag})
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
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
				results.Results = []params.StorageShowResult{
					params.StorageShowResult{Error: common.ServerError(errors.New(msg))},
				}
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.Show([]names.StorageTag{oneTag})
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
	c.Assert(called, jc.IsTrue)
}

func (s *storageMockSuite) TestList(c *gc.C) {
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
			c.Check(request, gc.Equals, "List")
			c.Check(a, gc.IsNil)

			if results, k := result.(*params.StorageListResult); k {
				results.Storages = []params.StorageInfo{
					params.StorageInfo{StorageTag: oneTag.String()},
					params.StorageInfo{
						StorageTag: twoTag.String(),
						Attached:   true,
					},
				}
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 2)
	c.Assert(expected.Contains(found[0].StorageTag), jc.IsTrue)
	c.Assert(expected.Contains(found[1].StorageTag), jc.IsTrue)
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
	found, err := storageClient.List()
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
	c.Assert(called, jc.IsTrue)
}
