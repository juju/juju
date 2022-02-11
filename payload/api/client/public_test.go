// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/api"
	"github.com/juju/juju/payload/api/client"
	"github.com/juju/juju/rpc/params"
)

type publicSuite struct {
	testing.IsolationSuite

	stub    *testing.Stub
	facade  *stubFacade
	payload params.Payload
}

var _ = gc.Suite(&publicSuite{})

func (s *publicSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.facade = &stubFacade{stub: s.stub}
	s.payload = params.Payload{
		Class:   "spam",
		Type:    "docker",
		ID:      "idspam",
		Status:  payload.StateRunning,
		Labels:  nil,
		Unit:    names.NewUnitTag("a-application/0").String(),
		Machine: names.NewMachineTag("1").String(),
	}
}

func (s *publicSuite) TestListOkay(c *gc.C) {
	s.facade.FacadeCallFn = func(_ string, _, response interface{}) error {
		typedResponse, ok := response.(*params.PayloadListResults)
		c.Assert(ok, gc.Equals, true)
		typedResponse.Results = append(typedResponse.Results, s.payload)
		return nil
	}

	pclient := client.NewPublicClient(s.facade)

	payloads, err := pclient.ListFull("a-tag", "a-application/0")
	c.Assert(err, jc.ErrorIsNil)

	expected, _ := api.API2Payload(s.payload)
	c.Check(payloads, jc.DeepEquals, []payload.FullPayloadInfo{
		expected,
	})
}

func (s *publicSuite) TestListAPI(c *gc.C) {
	pclient := client.NewPublicClient(s.facade)

	_, err := pclient.ListFull("a-tag")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "FacadeCall",
		Args: []interface{}{
			"List",
			&params.PayloadListArgs{
				Patterns: []string{"a-tag"},
			},
			&params.PayloadListResults{
				Results: nil,
			},
		},
	}})
}

type stubFacade struct {
	stub         *testing.Stub
	FacadeCallFn func(name string, params, response interface{}) error
}

func (s *stubFacade) FacadeCall(request string, params, response interface{}) error {
	s.stub.AddCall("FacadeCall", request, params, response)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if s.FacadeCallFn != nil {
		return s.FacadeCallFn(request, params, response)
	}
	return nil
}

func (s *stubFacade) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
