// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type unitStorageSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&unitStorageSuite{})

func (s *unitStorageSuite) createTestUnit(c *gc.C, t string, apiCaller basetesting.APICallerFunc) *uniter.Unit {
	tag := names.NewUnitTag(t)
	st := uniter.NewState(apiCaller, tag)
	return uniter.CreateUnit(st, tag)
}

func (s *unitStorageSuite) TestAddUnitStorage(c *gc.C) {
	count := uint64(1)
	args := map[string][]params.StorageConstraints{
		"data": []params.StorageConstraints{
			params.StorageConstraints{Count: &count}},
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
	u := s.createTestUnit(c, "mysql/0", apiCaller)
	err := u.AddStorage(args)
	c.Assert(err, gc.ErrorMatches, "yoink")
}

func (s *unitStorageSuite) TestAddUnitStorageError(c *gc.C) {
	count := uint64(1)
	args := map[string][]params.StorageConstraints{
		"data": []params.StorageConstraints{params.StorageConstraints{Count: &count}},
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

	u := s.createTestUnit(c, "mysql/0", apiCaller)
	err := u.AddStorage(args)
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}
