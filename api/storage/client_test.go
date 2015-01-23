// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/storage"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
)

type storageMockSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageMockSuite{})

func (s *storageMockSuite) TestShow(c *gc.C) {
	var called bool

	storageId := "shared-fs/0"
	storageTag := names.NewStorageTag(storageId).String()
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

			wanted, ok := a.(params.Entity)
			c.Assert(ok, jc.IsTrue)
			c.Assert(wanted.Tag, gc.DeepEquals, storageTag)
			if results, k := result.(*params.StorageInstancesResult); k {
				one := params.StorageInstance{StorageTag: storageTag}
				results.Results = []params.StorageInstance{one}
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.Show(storageId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0].StorageTag, gc.DeepEquals, storageTag)
	c.Assert(called, jc.IsTrue)
}
