// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/block"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type blockMockSuite struct {
	coretesting.BaseSuite
	blockClient *block.Client
}

var _ = gc.Suite(&blockMockSuite{})

func (s *blockMockSuite) TestSwitchBlockOn(c *gc.C) {
	called := false
	blockType := state.DestroyBlock.String()
	msg := "for test switch block on"

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, response interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Block")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "SwitchBlockOn")

			args, ok := a.(params.BlockSwitchParams)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Message, gc.DeepEquals, msg)
			c.Assert(args.Type, gc.DeepEquals, blockType)

			_, ok = response.(*params.ErrorResult)
			c.Assert(ok, jc.IsTrue)

			return nil
		})
	blockClient := block.NewClient(apiCaller)
	err := blockClient.SwitchBlockOn(blockType, msg)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.IsNil)
}

func (s *blockMockSuite) TestSwitchBlockOnError(c *gc.C) {
	called := false
	errmsg := "test error"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, response interface{},
		) error {
			called = true
			result, ok := response.(*params.ErrorResult)
			c.Assert(ok, jc.IsTrue)
			result.Error = common.ServerError(errors.New(errmsg))

			return nil
		})
	blockClient := block.NewClient(apiCaller)
	err := blockClient.SwitchBlockOn("", "")
	c.Assert(called, jc.IsTrue)
	c.Assert(errors.Cause(err), gc.ErrorMatches, errmsg)
}

func (s *blockMockSuite) TestSwitchBlockOff(c *gc.C) {
	called := false
	blockType := state.DestroyBlock.String()

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, response interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Block")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "SwitchBlockOff")

			args, ok := a.(params.BlockSwitchParams)
			c.Assert(ok, jc.IsTrue)
			// message is never sent, so this argument should
			// always be empty string.
			c.Assert(args.Message, gc.DeepEquals, "")
			c.Assert(args.Type, gc.DeepEquals, blockType)

			_, ok = response.(*params.ErrorResult)
			c.Assert(ok, jc.IsTrue)

			return nil
		})
	blockClient := block.NewClient(apiCaller)
	err := blockClient.SwitchBlockOff(blockType)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.IsNil)
}

func (s *blockMockSuite) TestSwitchBlockOffError(c *gc.C) {
	called := false
	errmsg := "test error"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, response interface{},
		) error {
			called = true
			result, ok := response.(*params.ErrorResult)
			c.Assert(ok, jc.IsTrue)
			result.Error = common.ServerError(errors.New(errmsg))

			return nil
		})
	blockClient := block.NewClient(apiCaller)
	err := blockClient.SwitchBlockOff("")
	c.Assert(called, jc.IsTrue)
	c.Assert(errors.Cause(err), gc.ErrorMatches, errmsg)
}

func (s *blockMockSuite) TestList(c *gc.C) {
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
		Error: common.ServerError(errors.New(errmsg)),
	}
	apiCaller := basetesting.APICallerFunc(
		func(
			objType string,
			version int,
			id, request string,
			a, response interface{}) error {
			called = true
			c.Check(objType, gc.Equals, "Block")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "List")
			c.Assert(a, gc.IsNil)

			result := response.(*params.BlockResults)
			result.Results = []params.BlockResult{one, two}
			return nil
		})
	blockClient := block.NewClient(apiCaller)
	found, err := blockClient.List()
	c.Assert(called, jc.IsTrue)
	c.Assert(errors.Cause(err), gc.ErrorMatches, errmsg)
	c.Assert(found, gc.HasLen, 1)
}
