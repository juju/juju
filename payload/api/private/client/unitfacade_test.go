// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/api"
	"github.com/juju/juju/payload/api/private/client"
)

type clientSuite struct {
	testing.IsolationSuite

	stub    *testing.Stub
	facade  *stubFacade
	payload params.Payload
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.facade = &stubFacade{stub: s.stub}
	s.facade.methods = &unitMethods{}
	s.payload = params.Payload{
		Class:  "foobar",
		Type:   "type",
		ID:     "idfoo",
		Status: payload.StateRunning,
	}

}

func (s *clientSuite) TestTrack(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	numStubCalls := 0
	s.facade.FacadeCallFn = func(name string, args, response interface{}) error {
		numStubCalls++
		c.Check(name, gc.Equals, "Track")

		typedResponse, ok := response.(*params.PayloadResults)
		c.Assert(ok, gc.Equals, true)
		typedResponse.Results = []params.PayloadResult{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id).String(),
			},
			Payload:  nil,
			NotFound: false,
			Error:    nil,
		}}
		return nil
	}

	pclient := client.NewUnitFacadeClient(s.facade)

	pl, err := api.API2Payload(s.payload)
	c.Assert(err, jc.ErrorIsNil)
	results, err := pclient.Track(pl.Payload)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID:       id,
		Payload:  nil,
		NotFound: false,
		Error:    nil,
	}})
}

func (s *clientSuite) TestList(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	responses := []interface{}{
		&params.PayloadResults{
			Results: []params.PayloadResult{{
				Entity: params.Entity{
					Tag: names.NewPayloadTag(id).String(),
				},
				Payload:  nil,
				NotFound: false,
				Error:    nil,
			}},
		},
		&params.PayloadResults{
			Results: []params.PayloadResult{{
				Entity: params.Entity{
					Tag: names.NewPayloadTag(id).String(),
				},
				Payload:  &s.payload,
				NotFound: false,
				Error:    nil,
			}},
		},
	}
	s.facade.responses = append(s.facade.responses, responses...)

	pclient := client.NewUnitFacadeClient(s.facade)

	results, err := pclient.List("idfoo/bar")
	c.Assert(err, jc.ErrorIsNil)

	expected, err := api.API2Payload(s.payload)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID:       id,
		Payload:  &expected,
		NotFound: false,
		Error:    nil,
	}})
	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "FacadeCall",
		Args: []interface{}{
			"LookUp",
			&params.LookUpPayloadArgs{
				Args: []params.LookUpPayloadArg{{
					Name: "idfoo",
					ID:   "bar",
				}},
			},
			responses[0],
		},
	}, {
		FuncName: "FacadeCall",
		Args: []interface{}{
			"List",
			&params.Entities{
				Entities: []params.Entity{{
					Tag: names.NewPayloadTag(id).String(),
				}},
			},
			responses[1],
		},
	}})
}

func (s *clientSuite) TestLookUpOkay(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	response := &params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id).String(),
			},
			Payload:  nil,
			NotFound: false,
			Error:    nil,
		}},
	}
	s.facade.responses = append(s.facade.responses, response)

	pclient := client.NewUnitFacadeClient(s.facade)
	results, err := pclient.LookUp("idfoo/bar")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID:       id,
		Payload:  nil,
		NotFound: false,
		Error:    nil,
	}})
	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "FacadeCall",
		Args: []interface{}{
			"LookUp",
			&params.LookUpPayloadArgs{
				Args: []params.LookUpPayloadArg{{
					Name: "idfoo",
					ID:   "bar",
				}},
			},
			response,
		},
	}})
}

func (s *clientSuite) TestLookUpMulti(c *gc.C) {
	id1 := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	id2 := "ce5bc2a7-65d8-4800-8199-a7c3356ab311"
	response := &params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id1).String(),
			},
			Payload:  nil,
			NotFound: false,
			Error:    nil,
		}, {
			Entity: params.Entity{
				Tag: "",
			},
			Payload:  nil,
			NotFound: true,
			Error:    common.ServerError(errors.NotFoundf("payload")),
		}, {
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id2).String(),
			},
			Payload:  nil,
			NotFound: false,
			Error:    nil,
		}},
	}
	s.facade.responses = append(s.facade.responses, response)

	pclient := client.NewUnitFacadeClient(s.facade)
	results, err := pclient.LookUp("idfoo/bar", "idbaz/bam", "spam/eggs")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 3)
	c.Assert(results[1].Error, gc.NotNil)
	results[1].Error = nil
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID:       id1,
		Payload:  nil,
		NotFound: false,
		Error:    nil,
	}, {
		ID:       "",
		Payload:  nil,
		NotFound: true,
		Error:    nil,
	}, {
		ID:       id2,
		Payload:  nil,
		NotFound: false,
		Error:    nil,
	}})
	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "FacadeCall",
		Args: []interface{}{
			"LookUp",
			&params.LookUpPayloadArgs{
				Args: []params.LookUpPayloadArg{{
					Name: "idfoo",
					ID:   "bar",
				}, {
					Name: "idbaz",
					ID:   "bam",
				}, {
					Name: "spam",
					ID:   "eggs",
				}},
			},
			response,
		},
	}})
}

func (s *clientSuite) TestSetStatus(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	responses := []interface{}{
		&params.PayloadResults{
			Results: []params.PayloadResult{{
				Entity: params.Entity{
					Tag: names.NewPayloadTag(id).String(),
				},
				Payload:  nil,
				NotFound: false,
				Error:    nil,
			}},
		},
		&params.PayloadResults{
			Results: []params.PayloadResult{{
				Entity: params.Entity{
					Tag: names.NewPayloadTag(id).String(),
				},
				Payload:  nil,
				NotFound: false,
				Error:    nil,
			}},
		},
	}
	s.facade.responses = append(s.facade.responses, responses...)

	pclient := client.NewUnitFacadeClient(s.facade)
	results, err := pclient.SetStatus(payload.StateRunning, "idfoo/bar")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID:       id,
		Payload:  nil,
		NotFound: false,
		Error:    nil,
	}})
	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "FacadeCall",
		Args: []interface{}{
			"LookUp",
			&params.LookUpPayloadArgs{
				Args: []params.LookUpPayloadArg{{
					Name: "idfoo",
					ID:   "bar",
				}},
			},
			responses[0],
		},
	}, {
		FuncName: "FacadeCall",
		Args: []interface{}{
			"SetStatus",
			&params.SetPayloadStatusArgs{
				Args: []params.SetPayloadStatusArg{{
					Entity: params.Entity{
						Tag: names.NewPayloadTag(id).String(),
					},
					Status: "running",
				}},
			},
			responses[1],
		},
	}})
}

func (s *clientSuite) TestUntrack(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	responses := []interface{}{
		&params.PayloadResults{
			Results: []params.PayloadResult{{
				Entity: params.Entity{
					Tag: names.NewPayloadTag(id).String(),
				},
				Payload:  nil,
				NotFound: false,
				Error:    nil,
			}},
		},
		&params.PayloadResults{
			Results: []params.PayloadResult{{
				Entity: params.Entity{
					Tag: names.NewPayloadTag(id).String(),
				},
				Payload:  nil,
				NotFound: false,
				Error:    nil,
			}},
		},
	}
	s.facade.responses = append(s.facade.responses, responses...)

	pclient := client.NewUnitFacadeClient(s.facade)
	results, err := pclient.Untrack("idfoo/bar")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID:       id,
		Payload:  nil,
		NotFound: false,
		Error:    nil,
	}})
	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "FacadeCall",
		Args: []interface{}{
			"LookUp",
			&params.LookUpPayloadArgs{
				Args: []params.LookUpPayloadArg{{
					Name: "idfoo",
					ID:   "bar",
				}},
			},
			responses[0],
		},
	}, {
		FuncName: "FacadeCall",
		Args: []interface{}{
			"Untrack",
			&params.Entities{
				Entities: []params.Entity{{
					Tag: names.NewPayloadTag(id).String(),
				}},
			},
			responses[1],
		},
	}})
}

type apiMethods interface {
	Handler(name string) (func(target, response interface{}), bool)
}

type stubFacade struct {
	stub      *testing.Stub
	responses []interface{}
	methods   apiMethods

	// TODO(ericsnow) Eliminate this.
	FacadeCallFn func(name string, params, response interface{}) error
}

func (s *stubFacade) nextResponse() interface{} {
	if len(s.responses) == 0 {
		return nil
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	return resp
}

func (s *stubFacade) FacadeCall(request string, params, response interface{}) error {
	s.stub.AddCall("FacadeCall", request, params, response)
	resp := s.nextResponse()
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if s.FacadeCallFn != nil {
		return s.FacadeCallFn(request, params, response)
	}

	if resp == nil {
		// TODO(ericsnow) Fail?
		return nil
	}
	handler, ok := s.methods.Handler(request)
	if !ok {
		return errors.Errorf("unknown request %q", request)
	}
	handler(response, resp)
	return nil
}

func (s *stubFacade) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type unitMethods struct{}

func (m unitMethods) Handler(name string) (func(target, response interface{}), bool) {
	switch name {
	case "List", "LookUp", "SetStatus", "Untrack":
		return m.generic, true
	default:
		return nil, false
	}
}

func (unitMethods) generic(target, response interface{}) {
	typedTarget := target.(*params.PayloadResults)
	typedResponse := response.(*params.PayloadResults)
	*typedTarget = *typedResponse
}
