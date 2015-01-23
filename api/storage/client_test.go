// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
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

	unitName := "test-unit"
	storageName := "test-storage"
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

			wanted, ok := a.(params.StorageInstance)
			c.Assert(ok, jc.IsTrue)
			c.Assert(wanted.UnitName, gc.DeepEquals, unitName)
			c.Assert(wanted.StorageName, gc.DeepEquals, storageName)
			if results, k := result.(*params.StorageInstancesResult); k {
				results.Results = []params.StorageInstance{wanted}
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.Show(unitName, storageName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0].UnitName, gc.DeepEquals, unitName)
	c.Assert(found[0].StorageName, gc.DeepEquals, storageName)
	c.Assert(called, jc.IsTrue)
}
