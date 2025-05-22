// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type modelInfoSuite struct {
	testing.BaseSuite
}

func TestModelInfoSuite(t *stdtesting.T) {
	tc.Run(t, &modelInfoSuite{})
}

func (s *modelInfoSuite) checkCall(c *tc.C, objType string, id, request string) {
	c.Check(objType, tc.Equals, "ModelManager")
	c.Check(id, tc.Equals, "")
	c.Check(request, tc.Equals, "ModelInfo")
}

func (s *modelInfoSuite) assertResponse(c *tc.C, result interface{}) *params.ModelInfoResults {
	c.Assert(result, tc.FitsTypeOf, &params.ModelInfoResults{})
	return result.(*params.ModelInfoResults)
}

func (s *modelInfoSuite) assertExpectedModelInfo(c *tc.C, expectedInfo params.ModelInfoResults) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			s.checkCall(c, objType, id, request)
			resp := s.assertResponse(c, result)
			*resp = expectedInfo
			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	input := []names.ModelTag{}
	for i := 0; i < len(expectedInfo.Results); i++ {
		input = append(input, testing.ModelTag)
	}
	info, err := client.ModelInfo(c.Context(), input)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, expectedInfo.Results)
}

func (s *modelInfoSuite) TestModelInfo(c *tc.C) {
	results := params.ModelInfoResults{
		Results: []params.ModelInfoResult{{
			Result: &params.ModelInfo{Name: "name", UUID: "etc.", Type: "foo"},
		}, {
			Error: &params.Error{Message: "woop"},
		}},
	}
	s.assertExpectedModelInfo(c, results)
}

func (s *modelInfoSuite) TestModelInfoOldController(c *tc.C) {
	results := params.ModelInfoResults{
		Results: []params.ModelInfoResult{{
			Result: &params.ModelInfo{Name: "name", UUID: "etc."},
		}, {
			Error: &params.Error{Message: "woop"},
		}},
	}
	s.assertExpectedModelInfo(c, results)
	c.Assert(results.Results[0].Result.Type, tc.Equals, "iaas")
}

func (s *modelInfoSuite) TestModelInfoWithAgentVersion(c *tc.C) {
	results := params.ModelInfoResults{
		Results: []params.ModelInfoResult{{
			Result: &params.ModelInfo{Name: "name", UUID: "etc.", Type: "foo", AgentVersion: &version.Current},
		}},
	}
	s.assertExpectedModelInfo(c, results)
}

func (s *modelInfoSuite) TestModelInfoWithSupportedFeatures(c *tc.C) {
	results := params.ModelInfoResults{
		Results: []params.ModelInfoResult{{
			Result: &params.ModelInfo{
				Name: "name",
				UUID: "etc.",
				Type: "foo",
				SupportedFeatures: []params.SupportedFeature{
					{Name: "foo", Description: "bar", Version: "2.9.17"},
				},
			},
		}},
	}
	s.assertExpectedModelInfo(c, results)
}

func (s *modelInfoSuite) TestInvalidResultCount(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			s.checkCall(c, objType, id, request)
			c.Assert(a, tc.DeepEquals, params.Entities{
				Entities: []params.Entity{{testing.ModelTag.String()}, {testing.ModelTag.String()}},
			})
			resp := s.assertResponse(c, result)
			*resp = params.ModelInfoResults{Results: []params.ModelInfoResult{{}}}
			return nil
		},
	)
	client := modelmanager.NewClient(apiCaller)
	_, err := client.ModelInfo(c.Context(), []names.ModelTag{testing.ModelTag, testing.ModelTag})
	c.Assert(err, tc.ErrorMatches, "expected 2 result\\(s\\), got 1")
}
