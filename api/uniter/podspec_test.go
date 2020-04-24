// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type podSpecSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&podSpecSuite{})

func strPtr(s string) *string {
	return &s
}

func (s *podSpecSuite) TestGetPodSpec(c *gc.C) {
	expected := params.Entities{
		Entities: []params.Entity{{
			Tag: "application-mysql",
		}},
	}

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(version, gc.Equals, 0)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "GetPodSpec")
		c.Assert(arg, gc.DeepEquals, expected)
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Error: &params.Error{Message: "yoink"},
			}},
		}
		return nil
	})
	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetPodSpec("mysql")
	c.Assert(err, gc.ErrorMatches, "yoink")
}

func (s *podSpecSuite) TestGetPodSpecInvalidApplicationName(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Fail()
		return nil
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetPodSpec("")
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
}

func (s *podSpecSuite) TestGetPodSpecError(c *gc.C) {
	expected := params.Entities{
		Entities: []params.Entity{{
			Tag: "application-mysql",
		}},
	}

	var called bool
	msg := "yoink"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(version, gc.Equals, 0)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "GetPodSpec")
		c.Assert(arg, gc.DeepEquals, expected)
		called = true

		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		return errors.New(msg)
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetPodSpec("mysql")
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}

func (s *podSpecSuite) TestGetPodSpecArity(c *gc.C) {
	expected := params.Entities{
		Entities: []params.Entity{{
			Tag: "application-mysql",
		}},
	}

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(version, gc.Equals, 0)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "GetPodSpec")
		c.Assert(arg, gc.DeepEquals, expected)
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{}, {}},
		}
		return nil
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetPodSpec("mysql")
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *podSpecSuite) TestGetRawK8sSpec(c *gc.C) {
	expected := params.Entities{
		Entities: []params.Entity{{
			Tag: "application-mysql",
		}},
	}

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(version, gc.Equals, 0)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "GetRawK8sSpec")
		c.Assert(arg, gc.DeepEquals, expected)
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Error: &params.Error{Message: "yoink"},
			}},
		}
		return nil
	})
	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetRawK8sSpec("mysql")
	c.Assert(err, gc.ErrorMatches, "yoink")
}

func (s *podSpecSuite) TestGetRawK8sSpecInvalidApplicationName(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Fail()
		return nil
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetRawK8sSpec("")
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
}

func (s *podSpecSuite) TestGetRawK8sSpecError(c *gc.C) {
	expected := params.Entities{
		Entities: []params.Entity{{
			Tag: "application-mysql",
		}},
	}

	var called bool
	msg := "yoink"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(version, gc.Equals, 0)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "GetRawK8sSpec")
		c.Assert(arg, gc.DeepEquals, expected)
		called = true

		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		return errors.New(msg)
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetRawK8sSpec("mysql")
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}

func (s *podSpecSuite) TestGetRawK8sSpecArity(c *gc.C) {
	expected := params.Entities{
		Entities: []params.Entity{{
			Tag: "application-mysql",
		}},
	}

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(version, gc.Equals, 0)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "GetRawK8sSpec")
		c.Assert(arg, gc.DeepEquals, expected)
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{}, {}},
		}
		return nil
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetRawK8sSpec("mysql")
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}
