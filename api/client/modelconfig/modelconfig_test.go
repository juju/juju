// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig_test

import (
	"github.com/golang/mock/gomock"
	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/modelconfig"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

type modelconfigSuite struct{}

var _ = gc.Suite(&modelconfigSuite{})

func (s *modelconfigSuite) TestModelGet(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.ModelConfigResults)
	results := params.ModelConfigResults{
		Config: map[string]params.ConfigValue{
			"foo": {"bar", "model"},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ModelGet", args, res).SetArg(2, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	result, err := client.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})
}

func (s *modelconfigSuite) TestModelGetWithMetadata(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.ModelConfigResults)
	results := params.ModelConfigResults{
		Config: map[string]params.ConfigValue{
			"foo": {"bar", "model"},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ModelGet", args, res).SetArg(2, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	result, err := client.ModelGetWithMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, config.ConfigValues{
		"foo": {"bar", "model"},
	})
}

func (s *modelconfigSuite) TestModelSet(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var res interface{}
	args := params.ModelSet{
		Config: map[string]interface{}{
			"some-name":  "value",
			"other-name": true,
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ModelSet", args, res).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.ModelSet(map[string]interface{}{
		"some-name":  "value",
		"other-name": true,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestModelUnset(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var res interface{}
	args := params.ModelUnset{
		Keys: []string{"foo", "bar"},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ModelUnset", args, res).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.ModelUnset("foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestSetSupport(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var res interface{}
	args := params.ModelSLA{
		ModelSLAInfo: params.ModelSLAInfo{
			Level: "foobar",
			Owner: "bob",
		},
		Credentials: []byte("creds"),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SetSLALevel", args, res).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.SetSLALevel("foobar", "bob", []byte("creds"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestGetSupport(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.StringResult)
	results := params.StringResult{
		Result: "level",
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SLALevel", args, res).SetArg(2, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	level, err := client.SLALevel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(level, gc.Equals, "level")
}

func (s *modelconfigSuite) TestSequences(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.ModelSequencesResult)
	results := params.ModelSequencesResult{
		Sequences: map[string]int{"foo": 5, "bar": 2},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Sequences", args, res).SetArg(2, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	sequences, err := client.Sequences()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sequences, jc.DeepEquals, map[string]int{"foo": 5, "bar": 2})
}

func (s *modelconfigSuite) TestGetModelConstraints(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.GetConstraintsResults)
	results := params.GetConstraintsResults{
		Constraints: constraints.MustParse("arch=amd64"),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("GetModelConstraints", args, res).SetArg(2, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	result, err := client.GetModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, constraints.MustParse("arch=amd64"))
}

func (s *modelconfigSuite) TestSetModelConstraints(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var res interface{}
	args := params.SetConstraints{
		Constraints: constraints.MustParse("arch=amd64"),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SetModelConstraints", args, res).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.SetModelConstraints(constraints.MustParse("arch=amd64"))
	c.Assert(err, jc.ErrorIsNil)
}
