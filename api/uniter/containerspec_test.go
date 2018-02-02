// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type containerSpecSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&containerSpecSuite{})

func (s *containerSpecSuite) TestSetContainerSpec(c *gc.C) {
	expected := params.SetContainerSpecParams{
		Entities: []params.EntityString{{
			Tag:   "application-mysql",
			Value: "spec",
		}},
	}

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(version, gc.Equals, expectedAPIVersion)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "SetContainerSpec")
		c.Assert(arg, gc.DeepEquals, expected)
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "yoink"},
			}},
		}
		return nil
	})
	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	err := st.SetContainerSpec("mysql", "spec")
	c.Assert(err, gc.ErrorMatches, "yoink")
}

func (s *containerSpecSuite) TestSetContainerSpecInvalidEntityame(c *gc.C) {
	st := uniter.NewState(nil, names.NewUnitTag("mysql/0"))
	err := st.SetContainerSpec("", "spec")
	c.Assert(err, gc.ErrorMatches, `application or unit name "" not valid`)
}

func (s *containerSpecSuite) TestSetContainerSpecError(c *gc.C) {
	expected := params.SetContainerSpecParams{
		Entities: []params.EntityString{{
			Tag:   "unit-mysql-0",
			Value: "spec",
		}},
	}

	var called bool
	msg := "yoink"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(version, gc.Equals, expectedAPIVersion)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "SetContainerSpec")
		c.Assert(arg, gc.DeepEquals, expected)
		called = true

		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		return errors.New(msg)
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	err := st.SetContainerSpec("mysql/0", "spec")
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}
