// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/names"
)

var _ = gc.Suite(&storageSuite{})

type storageSuite struct {
	coretesting.BaseSuite
}

func (s *storageSuite) TestStorageInstances(c *gc.C) {
	storageInstances := []params.UnitStorageInstances{
		{
			Instances: []storage.StorageInstance{
				{Id: "whatever", Kind: storage.StorageKindBlock, Location: "/dev/sda"},
			},
		},
	}

	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Uniter")
		c.Check(version, gc.Equals, 2)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "UnitStorageInstances")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-0"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.UnitStorageInstancesResults{})
		*(result.(*params.UnitStorageInstancesResults)) = params.UnitStorageInstancesResults{
			storageInstances,
		}
		called = true
		return nil
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	instances, err := st.StorageInstances(names.NewUnitTag("mysql/0"))
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
	c.Assert(instances, gc.DeepEquals, storageInstances[0].Instances)
}

func (s *storageSuite) TestStorageInstanceResultCountMismatch(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.UnitStorageInstancesResults)) = params.UnitStorageInstancesResults{
			[]params.UnitStorageInstances{{}, {}},
		}
		return nil
	})
	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	c.Assert(func() { st.StorageInstances(names.NewUnitTag("mysql/0")) }, gc.PanicMatches, "expected 1 result, got 2")
}

func (s *storageSuite) TestAPIErrors(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("bad")
	})
	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.StorageInstances(names.NewUnitTag("mysql/0"))
	c.Check(err, gc.ErrorMatches, "bad")
}
