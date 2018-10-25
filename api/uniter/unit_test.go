// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/juju/api/uniter"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type UnitSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&UnitSuite{})

func (s *UnitSuite) createTestUnit(c *gc.C, t string, apiCaller basetesting.APICallerFunc) *uniter.Unit {
	tag := names.NewUnitTag(t)
	st := uniter.NewState(apiCaller, tag)
	return uniter.CreateUnit(st, tag)
}

func (s *UnitSuite) TestRefreshSetHasAddress(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(version, gc.Equals, expectedAPIVersion)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "Refresh")
		c.Assert(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{
				{Tag: "unit-mysql-0"},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.UnitRefreshResults{})
		*(result.(*params.UnitRefreshResults)) = params.UnitRefreshResults{
			Results: []params.UnitRefreshResult{{
				HasAddress: true,
			}},
		}
		return nil
	})
	u := s.createTestUnit(c, "mysql/0", apiCaller)
	err := u.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.HasAddress(), jc.IsTrue)
}
