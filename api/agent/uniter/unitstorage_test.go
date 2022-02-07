// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type unitStorageSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&unitStorageSuite{})

func (s *unitStorageSuite) TestAddUnitStorage(c *gc.C) {
	count := uint64(1)
	args := map[string][]params.StorageConstraints{
		"data": {
			{Count: &count}},
	}

	expected := params.StoragesAddParams{
		Storages: []params.StorageAddParams{
			{"unit-mysql-0", "data", params.StorageConstraints{Count: &count}},
		},
	}

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(version, gc.Equals, 2)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "AddUnitStorage")
		c.Assert(arg, gc.DeepEquals, expected)
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "yoink"},
			}},
		}
		return nil
	})
	caller := basetesting.BestVersionCaller{apiCaller, 2}
	tag := names.NewUnitTag("mysql/0")
	st := uniter.NewState(caller, tag)
	u := uniter.CreateUnit(st, tag)
	err := u.AddStorage(args)
	c.Assert(err, gc.ErrorMatches, "yoink")
}

func (s *unitStorageSuite) TestAddUnitStorageError(c *gc.C) {
	count := uint64(1)
	args := map[string][]params.StorageConstraints{
		"data": {{Count: &count}},
	}

	expected := params.StoragesAddParams{
		Storages: []params.StorageAddParams{
			{"unit-mysql-0", "data", params.StorageConstraints{Count: &count}},
		},
	}

	var called bool
	msg := "yoink"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(version, gc.Equals, 2)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "AddUnitStorage")
		c.Assert(arg, gc.DeepEquals, expected)
		called = true

		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		return errors.New(msg)
	})

	caller := basetesting.BestVersionCaller{apiCaller, 2}
	tag := names.NewUnitTag("mysql/0")
	st := uniter.NewState(caller, tag)
	u := uniter.CreateUnit(st, tag)
	err := u.AddStorage(args)
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}
