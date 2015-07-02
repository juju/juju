package client_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process/api"
	"github.com/juju/juju/process/api/client"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type clientSuite struct {
	stub        *stubFacade
	tag         string
	processInfo api.ProcessInfo
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.tag = "machine-tag"
	s.stub = &stubFacade{}
	s.processInfo = api.ProcessInfo{
		Process: api.Process{
			Name:        "foobar",
			Description: "desc",
			Type:        "type",
			TypeOptions: map[string]string{"foo": "bar"},
			Command:     "cmd",
			Image:       "img",
			Ports: []api.ProcessPort{{
				External: 8080,
				Internal: 80,
				Endpoint: "endpoint",
			}},
			Volumes: []api.ProcessVolume{{
				ExternalMount: "/foo/bar",
				InternalMount: "/baz/bat",
				Mode:          "ro",
				Name:          "volname",
			}},
			EnvVars: map[string]string{"envfoo": "bar"},
		},
		Status: 5,
		Details: api.ProcDetails{
			ID:         "idfoo",
			ProcStatus: api.ProcStatus{Status: "process status"},
		},
	}

}

func (s *clientSuite) TestRegisterProcesses(c *gc.C) {
	numStubCalls := 0
	s.stub.FacadeCallFn = func(name string, params, response interface{}) error {
		numStubCalls++
		c.Check(name, gc.Equals, "RegisterProcesses")

		typedResponse, ok := response.(*api.ProcessResults)
		c.Assert(ok, gc.Equals, true)

		typedParams, ok := params.(*api.RegisterProcessesArgs)
		c.Assert(ok, gc.Equals, true)

		for _, rpa := range typedParams.Processes {
			typedResponse.Results = append(typedResponse.Results, api.ProcessResult{
				ID:    rpa.ProcessInfo.Details.ID,
				Error: nil,
			})
		}

		return nil
	}

	pclient := client.NewProcessClient(s.stub, s.stub)

	unregisteredProcesses := []api.ProcessInfo{s.processInfo}

	ids, err := pclient.RegisterProcesses(s.tag, unregisteredProcesses)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(len(ids), gc.Equals, 1)
	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(ids[0], gc.Equals, api.ProcessResult{ID: s.processInfo.Details.ID, Error: nil})
}

func (s *clientSuite) TestListAllProcesses(c *gc.C) {
	numStubCalls := 0

	s.stub.FacadeCallFn = func(name string, params, response interface{}) error {
		numStubCalls++
		c.Check(name, gc.Equals, "ListProcesses")

		typedResponse, ok := response.(*api.ListProcessesResults)
		c.Assert(ok, gc.Equals, true)

		result := api.ListProcessResult{ID: s.processInfo.Details.ID, Info: s.processInfo, Error: nil}
		typedResponse.Results = append(typedResponse.Results, result)

		return nil
	}
	pclient := client.NewProcessClient(s.stub, s.stub)

	processes, err := pclient.ListProcesses(s.tag)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(len(processes), gc.Equals, 1)
	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(processes[0], gc.DeepEquals, api.ListProcessResult{ID: "idfoo", Info: s.processInfo, Error: nil})
}

func (s *clientSuite) TestSetProcessesStatus(c *gc.C) {
	numStubCalls := 0

	s.stub.FacadeCallFn = func(name string, params, response interface{}) error {
		numStubCalls++
		c.Check(name, gc.Equals, "SetProcessesStatus")

		typedParams, ok := params.(*api.SetProcessesStatusArgs)
		c.Assert(ok, gc.Equals, true)

		c.Check(len(typedParams.Args), gc.Equals, 1)

		arg := typedParams.Args[0]
		c.Check(arg.UnitTag, gc.Equals, s.tag)
		c.Check(arg.ID, gc.Equals, "idfoo")
		c.Check(arg.Status, gc.DeepEquals, api.ProcStatus{Status: "Running"})

		return nil
	}

	pclient := client.NewProcessClient(s.stub, s.stub)
	err := pclient.SetProcessesStatus(s.tag, "Running", "idfoo")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(numStubCalls, gc.Equals, 1)
}

func (s *clientSuite) TestUnregisterProcesses(c *gc.C) {
	numStubCalls := 0

	s.stub.FacadeCallFn = func(name string, params, response interface{}) error {
		numStubCalls++
		c.Check(name, gc.Equals, "UnregisterProcesses")

		typedParams, ok := params.(*api.UnregisterProcessesArgs)
		c.Assert(ok, gc.Equals, true)

		c.Check(len(typedParams.IDs), gc.Equals, 1)
		c.Check(typedParams.UnitTag, gc.Equals, s.tag)

		return nil
	}

	pclient := client.NewProcessClient(s.stub, s.stub)
	err := pclient.UnregisterProcesses(s.tag, "idfoo")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(numStubCalls, gc.Equals, 1)
}

type stubFacade struct {
	FacadeCallFn func(name string, params, response interface{}) error
}

func (s *stubFacade) FacadeCall(request string, params, response interface{}) error {
	if s.FacadeCallFn != nil {
		return s.FacadeCallFn(request, params, response)
	}
	return nil
}

func (s *stubFacade) BestAPIVersion() int {
	return -1
}

func (s *stubFacade) Close() error {
	return nil
}
