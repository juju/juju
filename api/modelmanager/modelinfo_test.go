// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
)

type modelInfoSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&modelInfoSuite{})

func (s *modelInfoSuite) checkCall(c *gc.C, objType string, id, request string) {
	c.Check(objType, gc.Equals, "ModelManager")
	c.Check(id, gc.Equals, "")
	c.Check(request, gc.Equals, "ModelInfo")
}

func (s *modelInfoSuite) assertResponse(c *gc.C, result interface{}) *params.ModelInfoResults {
	c.Assert(result, gc.FitsTypeOf, &params.ModelInfoResults{})
	return result.(*params.ModelInfoResults)
}

func (s *modelInfoSuite) TestModelInfo(c *gc.C) {
	results := params.ModelInfoResults{
		Results: []params.ModelInfoResult{{
			Result: &params.ModelInfo{Name: "name", UUID: "etc."},
		}, {
			Error: &params.Error{Message: "woop"},
		}},
	}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			s.checkCall(c, objType, id, request)
			resp := s.assertResponse(c, result)
			*resp = results
			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	info, err := client.ModelInfo([]names.ModelTag{testing.ModelTag, testing.ModelTag})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, results.Results)
}

func (s *modelInfoSuite) TestInvalidResultCount(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			s.checkCall(c, objType, id, request)
			c.Assert(a, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{{testing.ModelTag.String()}, {testing.ModelTag.String()}},
			})
			resp := s.assertResponse(c, result)
			*resp = params.ModelInfoResults{Results: []params.ModelInfoResult{{}}}
			return nil
		},
	)
	client := modelmanager.NewClient(apiCaller)
	_, err := client.ModelInfo([]names.ModelTag{testing.ModelTag, testing.ModelTag})
	c.Assert(err, gc.ErrorMatches, "expected 2 result\\(s\\), got 1")
}
