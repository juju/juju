// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package private_test

import (
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/api/private"
)

type internalHelpersSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&internalHelpersSuite{})

func (internalHelpersSuite) TestAPI2ResultOkay(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	result, err := private.API2Result(params.PayloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		Payload:  nil,
		NotFound: false,
		Error:    nil,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, jc.DeepEquals, payload.Result{
		ID:       id,
		Payload:  nil,
		NotFound: false,
		Error:    nil,
	})
}

func (internalHelpersSuite) TestAPI2ResultInfo(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	result, err := private.API2Result(params.PayloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		NotFound: false,
		Error:    nil,
		Payload: &params.Payload{
			Class:   "foobar",
			Type:    "type",
			ID:      "idfoo",
			Status:  payload.StateRunning,
			Unit:    "unit-a-application-0",
			Machine: "machine-1",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, jc.DeepEquals, payload.Result{
		ID:       id,
		NotFound: false,
		Error:    nil,
		Payload: &payload.FullPayloadInfo{
			Payload: payload.Payload{
				PayloadClass: charm.PayloadClass{
					Name: "foobar",
					Type: "type",
				},
				ID:     "idfoo",
				Status: payload.StateRunning,
				Unit:   "a-application/0",
			},
			Machine: "1",
		},
	})
}

func (internalHelpersSuite) TestAPI2ResultError(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	failure := errors.New("<failure>")
	result, err := private.API2Result(params.PayloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		Payload:  nil,
		NotFound: false,
		Error:    common.ServerError(failure),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result.Error.Error(), gc.Equals, failure.Error())
	c.Check(result, jc.DeepEquals, payload.Result{
		ID:       id,
		Payload:  nil,
		NotFound: false,
		Error:    result.Error, // The actual error is checked above.
	})
}

func (internalHelpersSuite) TestAPI2ResultNotFound(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	notFound := errors.NotFoundf("payload %q", id)
	result, err := private.API2Result(params.PayloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		Payload:  nil,
		NotFound: false,
		Error:    common.ServerError(notFound),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result.Error.Error(), gc.Equals, notFound.Error())
	c.Check(result.Error, jc.Satisfies, errors.IsNotFound)
	c.Check(result, jc.DeepEquals, payload.Result{
		ID:       id,
		Payload:  nil,
		NotFound: false,
		Error:    result.Error, // The actual error is checked above.
	})
}
