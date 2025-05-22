// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/block"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

type blockMockSuite struct{}

func TestBlockMockSuite(t *testing.T) {
	tc.Run(t, &blockMockSuite{})
}

func (s *blockMockSuite) TestSwitchBlockOn(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	blockType := params.BlockDestroy
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
	err := blockClient.SwitchBlockOn(c.Context(), blockType, msg)
	c.Assert(err, tc.IsNil)
}

func (s *blockMockSuite) TestSwitchBlockOnError(c *tc.C) {
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
	err := blockClient.SwitchBlockOn(c.Context(), "", "")
	c.Assert(errors.Cause(err), tc.ErrorMatches, errmsg)
}

func (s *blockMockSuite) TestSwitchBlockOff(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	blockType := params.BlockDestroy

	args := params.BlockSwitchParams{
		Type:    blockType,
		Message: "",
	}
	result := new(params.ErrorResult)
	results := params.ErrorResult{Error: nil}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SwitchBlockOff", args, result).SetArg(3, results).Return(nil)

	blockClient := block.NewClientFromCaller(mockFacadeCaller)
	err := blockClient.SwitchBlockOff(c.Context(), blockType)
	c.Assert(err, tc.IsNil)
}

func (s *blockMockSuite) TestSwitchBlockOffError(c *tc.C) {
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
	err := blockClient.SwitchBlockOff(c.Context(), "")
	c.Assert(errors.Cause(err), tc.ErrorMatches, errmsg)
}

func (s *blockMockSuite) TestList(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	one := params.BlockResult{
		Result: params.Block{
			Id:      "-42",
			Type:    params.BlockDestroy,
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
	found, err := blockClient.List(c.Context())
	c.Assert(errors.Cause(err), tc.ErrorMatches, errmsg)
	c.Assert(found, tc.HasLen, 1)
}
