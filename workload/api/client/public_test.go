package client_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api"
	"github.com/juju/juju/workload/api/client"
)

type publicSuite struct {
	stub    *testing.Stub
	facade  *stubFacade
	payload api.Payload
}

var _ = gc.Suite(&publicSuite{})

func (s *publicSuite) SetUpTest(c *gc.C) {
	s.stub = &testing.Stub{}
	s.facade = &stubFacade{stub: s.stub}
	s.payload = api.Payload{
		Class:   "spam",
		Type:    "docker",
		ID:      "idspam",
		Status:  workload.StateRunning,
		Tags:    nil,
		Unit:    "unit-a-service-0",
		Machine: "1",
	}
}

func (s *publicSuite) TestListOkay(c *gc.C) {
	s.facade.FacadeCallFn = func(_ string, _, response interface{}) error {
		typedResponse, ok := response.(*api.EnvListResults)
		c.Assert(ok, gc.Equals, true)
		typedResponse.Results = append(typedResponse.Results, s.payload)
		return nil
	}

	pclient := client.NewPublicClient(s.facade)

	payloads, err := pclient.List("a-tag", "unit-a-service-0")
	c.Assert(err, jc.ErrorIsNil)

	expected := api.API2Payload(s.payload)
	c.Check(payloads, jc.DeepEquals, []workload.Payload{expected})
}

func (s *publicSuite) TestListAPI(c *gc.C) {
	pclient := client.NewPublicClient(s.facade)

	_, err := pclient.List("a-tag")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "FacadeCall",
		Args: []interface{}{
			"List",
			&api.EnvListArgs{
				Patterns: []string{"a-tag"},
			},
			&api.EnvListResults{
				Results: nil,
			},
		},
	}})
}
