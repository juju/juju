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

func (s *modelconfigSuite) TestSetSupport(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelConfig")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "SetSLALevel")
			c.Check(a, jc.DeepEquals, params.ModelSLA{
				ModelSLAInfo: params.ModelSLAInfo{
					Level: "foobar",
					Owner: "bob",
				},
				Credentials: []byte("creds"),
			})
			called = true
			return nil
		},
	)
	client := modelconfig.NewClient(apiCaller)
	err := client.SetSLALevel("foobar", "bob", []byte("creds"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *modelconfigSuite) TestGetSupport(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelConfig")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "SLALevel")
			c.Check(a, jc.DeepEquals, nil)
			results := result.(*params.StringResult)
			results.Result = "level"
			called = true
			return nil
		},
	)
	client := modelconfig.NewClient(apiCaller)
	level, err := client.SLALevel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(level, gc.Equals, "level")
}

func (s *modelconfigSuite) TestSequencesV1(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		basetesting.APICallerFunc(
			func(_ string, _ int, _, _ string, _, _ interface{}) error {
				c.Errorf("shoulnd't be called")
				return nil
			},
		), 1} // facade version one
	client := modelconfig.NewClient(apiCaller)
	sequences, err := client.Sequences()
	c.Assert(err, gc.ErrorMatches, "Sequences on v1 facade not supported")
	c.Assert(sequences, gc.IsNil)
}

func (s *modelconfigSuite) TestSequences(c *gc.C) {
	called := false
	apiCaller := basetesting.BestVersionCaller{
		basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "ModelConfig")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "Sequences")
				c.Check(a, jc.DeepEquals, nil)
				results := result.(*params.ModelSequencesResult)
				results.Sequences = map[string]int{"foo": 5, "bar": 2}
				called = true
				return nil
			},
		), 2}
	client := modelconfig.NewClient(apiCaller)
	sequences, err := client.Sequences()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(sequences, jc.DeepEquals, map[string]int{"foo": 5, "bar": 2})
}
