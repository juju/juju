// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/block"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type blockMockSuite struct {
}

var _ = gc.Suite(&blockMockSuite{})

func (s *blockMockSuite) TestSwitchBlockOn(c *gc.C) {
	ctrl := gomock.NewController(c)
	called := false
	blockType := state.DestroyBlock.String()
	msg := "for test switch block on"

	args := params.BlockSwitchParams{
		Type:    blockType,
		Message: msg,
	}
	result := new(params.ErrorResult)
	results := params.ErrorResult{Error: nil}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SwitchBlockOn", args, result).SetArg(2, results).DoAndReturn(
		func(arg0 string, args params.BlockSwitchParams, results *params.ErrorResult) error {
			called = true
			c.Assert(args.Message, gc.DeepEquals, msg)
			c.Assert(args.Type, gc.DeepEquals, blockType)
			return nil
		})

	blockClient := block.NewClientFromCaller(mockFacadeCaller)
	err := blockClient.SwitchBlockOn(blockType, msg)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.IsNil)
}

func (s *blockMockSuite) TestSwitchBlockOnError(c *gc.C) {
	ctrl := gomock.NewController(c)
	called := false
	errmsg := "test error"

	args := params.BlockSwitchParams{
		Type:    "",
		Message: "",
	}
	result := new(params.ErrorResult)
	results := params.ErrorResult{
		Error: apiservererrors.ServerError(errors.New(errmsg)),
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SwitchBlockOn", args, result).SetArg(2, results).DoAndReturn(
		func(arg0 string, args params.BlockSwitchParams, results *params.ErrorResult) error {
			called = true
			return nil
		})

	blockClient := block.NewClientFromCaller(mockFacadeCaller)
	err := blockClient.SwitchBlockOn("", "")
	c.Assert(called, jc.IsTrue)
	c.Assert(errors.Cause(err), gc.ErrorMatches, errmsg)
}

func (s *blockMockSuite) TestSwitchBlockOff(c *gc.C) {
	ctrl := gomock.NewController(c)
	called := false
	blockType := state.DestroyBlock.String()

	args := params.BlockSwitchParams{
		Type:    blockType,
		Message: "",
	}
	result := new(params.ErrorResult)
	results := params.ErrorResult{Error: nil}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SwitchBlockOff", args, result).SetArg(2, results).DoAndReturn(
		func(arg0 string, args params.BlockSwitchParams, results *params.ErrorResult) error {
			called = true
			// message is never sent, so this argument should
			// always be empty string.
			c.Assert(args.Message, gc.DeepEquals, "")
			c.Assert(args.Type, gc.DeepEquals, blockType)
			return nil
		})

	blockClient := block.NewClientFromCaller(mockFacadeCaller)
	err := blockClient.SwitchBlockOff(blockType)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.IsNil)
}

func (s *blockMockSuite) TestSwitchBlockOffError(c *gc.C) {
	ctrl := gomock.NewController(c)
	called := false
	errmsg := "test error"

	args := params.BlockSwitchParams{
		Type: "",
	}
	result := new(params.ErrorResult)
	results := params.ErrorResult{
		Error: apiservererrors.ServerError(errors.New(errmsg)),
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SwitchBlockOff", args, result).SetArg(2, results).DoAndReturn(
		func(arg0 string, args params.BlockSwitchParams, results *params.ErrorResult) error {
			called = true
			return nil
		})

	blockClient := block.NewClientFromCaller(mockFacadeCaller)
	err := blockClient.SwitchBlockOff("")
	c.Assert(called, jc.IsTrue)
	c.Assert(errors.Cause(err), gc.ErrorMatches, errmsg)
}

func (s *blockMockSuite) TestList(c *gc.C) {
	ctrl := gomock.NewController(c)
	var called bool
	one := params.BlockResult{
		Result: params.Block{
			Id:      "-42",
			Type:    state.DestroyBlock.String(),
			Message: "for test switch on",
			Tag:     "some valid tag, right?",
		},
	}
	errmsg := "another test error"
	two := params.BlockResult{
		Error: apiservererrors.ServerError(errors.New(errmsg)),
	}

	result := new(params.BlockResults)
	results := params.BlockResults{
		Results: []params.BlockResult{one, two},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("List", nil, result).SetArg(2, results).DoAndReturn(
		func(arg0 string, args interface{}, results *params.BlockResults) (*params.BlockResults, error) {
			called = true
			return results, apiservererrors.ServerError(errors.New(errmsg))
		})
	blockClient := block.NewClientFromCaller(mockFacadeCaller)
	found, err := blockClient.List()
	c.Assert(called, jc.IsTrue)
	c.Assert(errors.Cause(err), gc.ErrorMatches, errmsg)
	c.Assert(found, gc.HasLen, 1)
}
