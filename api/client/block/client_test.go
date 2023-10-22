// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	"github.com/juju/errors"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/block"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

type blockMockSuite struct{}

var _ = gc.Suite(&blockMockSuite{})

func (s *blockMockSuite) TestSwitchBlockOn(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	blockType := string(model.BlockDestroy)
	msg := "for test switch block on"

	args := params.BlockSwitchParams{
		Type:    blockType,
		Message: msg,
	}
	result := new(params.ErrorResult)
	results := params.ErrorResult{Error: nil}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SwitchBlockOn", args, result).SetArg(3, results).Return(nil)

	blockClient := block.NewClientFromCaller(mockFacadeCaller)
	err := blockClient.SwitchBlockOn(blockType, msg)
	c.Assert(err, gc.IsNil)
}

func (s *blockMockSuite) TestSwitchBlockOnError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SwitchBlockOn", args, result).SetArg(3, results).Return(nil)

	blockClient := block.NewClientFromCaller(mockFacadeCaller)
	err := blockClient.SwitchBlockOn("", "")
	c.Assert(errors.Cause(err), gc.ErrorMatches, errmsg)
}

func (s *blockMockSuite) TestSwitchBlockOff(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	blockType := string(model.BlockDestroy)

	args := params.BlockSwitchParams{
		Type:    blockType,
		Message: "",
	}
	result := new(params.ErrorResult)
	results := params.ErrorResult{Error: nil}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SwitchBlockOff", args, result).SetArg(3, results).Return(nil)

	blockClient := block.NewClientFromCaller(mockFacadeCaller)
	err := blockClient.SwitchBlockOff(blockType)
	c.Assert(err, gc.IsNil)
}

func (s *blockMockSuite) TestSwitchBlockOffError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	errmsg := "test error"

	args := params.BlockSwitchParams{
		Type: "",
	}
	result := new(params.ErrorResult)
	results := params.ErrorResult{
		Error: apiservererrors.ServerError(errors.New(errmsg)),
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SwitchBlockOff", args, result).SetArg(3, results).Return(nil)

	blockClient := block.NewClientFromCaller(mockFacadeCaller)
	err := blockClient.SwitchBlockOff("")
	c.Assert(errors.Cause(err), gc.ErrorMatches, errmsg)
}

func (s *blockMockSuite) TestList(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	one := params.BlockResult{
		Result: params.Block{
			Id:      "-42",
			Type:    string(model.BlockDestroy),
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "List", nil, result).SetArg(3, results).Return(nil)
	blockClient := block.NewClientFromCaller(mockFacadeCaller)
	found, err := blockClient.List()
	c.Assert(errors.Cause(err), gc.ErrorMatches, errmsg)
	c.Assert(found, gc.HasLen, 1)
}
