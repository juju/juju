// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
)

type modelconfigSuite struct {
	gitjujutesting.IsolationSuite
}

var _ = gc.Suite(&modelconfigSuite{})

func (s *modelconfigSuite) TestModelGet(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelConfig")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ModelGet")
			c.Check(a, gc.IsNil)
			c.Assert(result, gc.FitsTypeOf, &params.ModelConfigResults{})
			results := result.(*params.ModelConfigResults)
			results.Config = map[string]params.ConfigValue{
				"foo": {"bar", "model"},
			}
			return nil
		},
	)
	client := modelconfig.NewClient(apiCaller)
	result, err := client.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})
}

func (s *modelconfigSuite) TestModelGetWithMetadata(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelConfig")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ModelGet")
			c.Check(a, gc.IsNil)
			c.Assert(result, gc.FitsTypeOf, &params.ModelConfigResults{})
			results := result.(*params.ModelConfigResults)
			results.Config = map[string]params.ConfigValue{
				"foo": {"bar", "model"},
			}
			return nil
		},
	)
	client := modelconfig.NewClient(apiCaller)
	result, err := client.ModelGetWithMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, config.ConfigValues{
		"foo": {"bar", "model"},
	})
}

func (s *modelconfigSuite) TestModelSet(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelConfig")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ModelSet")
			c.Check(a, jc.DeepEquals, params.ModelSet{
				Config: map[string]interface{}{
					"some-name":  "value",
					"other-name": true,
				},
			})
			called = true
			return nil
		},
	)
	client := modelconfig.NewClient(apiCaller)
	err := client.ModelSet(map[string]interface{}{
		"some-name":  "value",
		"other-name": true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *modelconfigSuite) TestModelUnset(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelConfig")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ModelUnset")
			c.Check(a, jc.DeepEquals, params.ModelUnset{
				Keys: []string{"foo", "bar"},
			})
			called = true
			return nil
		},
	)
	client := modelconfig.NewClient(apiCaller)
	err := client.ModelUnset("foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *modelconfigSuite) TestModelDefaults(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelConfig")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ModelDefaults")
			c.Check(a, gc.IsNil)
			c.Assert(result, gc.FitsTypeOf, &params.ModelDefaultsResult{})
			results := result.(*params.ModelDefaultsResult)
			results.Config = map[string]params.ModelDefaults{
				"foo": {"bar", "model", []params.RegionDefaults{{
					"dummy-region",
					"dummy-value"}}},
			}
			return nil
		},
	)
	client := modelconfig.NewClient(apiCaller)
	result, err := client.ModelDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, jc.DeepEquals, config.ModelDefaultAttributes{
		"foo": {"bar", "model", []config.RegionDefaultValue{{
			"dummy-region",
			"dummy-value"}}},
	})
}

func (s *modelconfigSuite) TestSetModelDefaults(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelConfig")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "SetModelDefaults")
			c.Check(a, jc.DeepEquals, params.SetModelDefaults{
				Config: []params.ModelDefaultValues{{
					CloudTag:    "cloud-mycloud",
					CloudRegion: "region",
					Config: map[string]interface{}{
						"some-name":  "value",
						"other-name": true,
					},
				}}})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{Error: nil}},
			}
			called = true
			return nil
		},
	)
	client := modelconfig.NewClient(apiCaller)
	err := client.SetModelDefaults("mycloud", "region", map[string]interface{}{
		"some-name":  "value",
		"other-name": true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *modelconfigSuite) TestUnsetModelDefaults(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelConfig")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "UnsetModelDefaults")
			c.Check(a, jc.DeepEquals, params.UnsetModelDefaults{
				Keys: []params.ModelUnsetKeys{{
					CloudTag:    "cloud-mycloud",
					CloudRegion: "region",
					Keys:        []string{"foo", "bar"},
				}}})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{Error: nil}},
			}
			called = true
			return nil
		},
	)
	client := modelconfig.NewClient(apiCaller)
	err := client.UnsetModelDefaults("mycloud", "region", "foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}
