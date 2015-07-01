package client_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	papi "github.com/juju/juju/process/api"
	pclient "github.com/juju/juju/process/api/client"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type clientSuite struct {
	stub *stubFacade
	tag  string
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.tag = "machine-tag"
	s.stub = &stubFacade{}
}

func (s *clientSuite) TestRegisterProcesses(c *gc.C) {
	numStubCalls := 0
	s.stub.FacadeCallFn = func(name string, params, response interface{}) error {
		numStubCalls++
		c.Check(name, gc.Equals, "RegisterProcesses")

		typedResponse, ok := response.(*papi.ProcessResults)
		c.Assert(ok, gc.Equals, true)

		typedParams, ok := params.(*papi.RegisterProcessesArgs)
		c.Assert(ok, gc.Equals, true)

		for _, rpa := range typedParams.Processes {
			typedResponse.Results = append(typedResponse.Results, papi.ProcessResult{
				ID:    rpa.ProcessInfo.Details.ID,
				Error: nil,
			})
		}

		return nil
	}

	client := pclient.NewProcessClient(s.stub, s.stub)

	processInfo := papi.ProcessInfo{
		Process: papi.Process{
			Name:        "foobar",
			Description: "desc",
			Type:        "type",
			TypeOptions: map[string]string{"foo": "bar"},
			Command:     "cmd",
			Image:       "img",
			Ports: []papi.ProcessPort{{
				External: 8080,
				Internal: 80,
				Endpoint: "endpoint",
			}},
			Volumes: []papi.ProcessVolume{{
				ExternalMount: "/foo/bar",
				InternalMount: "/baz/bat",
				Mode:          "ro",
				Name:          "volname",
			}},
			EnvVars: map[string]string{"envfoo": "bar"},
		},
		Status: 5,
		Details: papi.ProcDetails{
			ID:         "idfoo",
			ProcStatus: papi.ProcStatus{Status: "process status"},
		},
	}

	unregisteredProcesses := []papi.ProcessInfo{processInfo}

	ids, err := client.RegisterProcesses(s.tag, unregisteredProcesses)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(len(ids), gc.Equals, 1)
	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(ids[0], gc.Equals, processInfo.Details.ID)
}

func (s *clientSuite) TestListEmptyProcesses(c *gc.C) {
	numStubCalls := 0

	s.stub.FacadeCallFn = func(name string, params, response interface{}) error {
		numStubCalls++
		c.Check(name, gc.Equals, "ListProcesses")
		return nil
	}
	client := pclient.NewProcessClient(s.stub, s.stub)

	processes, err := client.ListProcesses(s.tag)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(len(processes), gc.Equals, 0)
	c.Check(numStubCalls, gc.Equals, 1)
}

func (s *clientSuite) TestSetProcessesStatus(c *gc.C) {
	numStubCalls := 0

	s.stub.FacadeCallFn = func(name string, params, response interface{}) error {
		numStubCalls++
		c.Check(name, gc.Equals, "SetProcessesStatus")

		typedParams, ok := params.(*papi.SetProcessesStatusArgs)
		c.Assert(ok, gc.Equals, true)

		c.Check(len(typedParams.Args), gc.Equals, 1)

		arg := typedParams.Args[0]
		c.Check(arg.UnitTag, gc.Equals, s.tag)
		c.Check(arg.ID, gc.Equals, "idfoo")
		c.Check(arg.Status, gc.DeepEquals, papi.ProcStatus{Status: "Running"})

		return nil
	}

	client := pclient.NewProcessClient(s.stub, s.stub)
	err := client.SetProcessesStatus(s.tag, "Running", "idfoo")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(numStubCalls, gc.Equals, 1)
}

func (s *clientSuite) TestUnregisterProcesses(c *gc.C) {
	numStubCalls := 0

	s.stub.FacadeCallFn = func(name string, params, response interface{}) error {
		numStubCalls++
		c.Check(name, gc.Equals, "UnregisterProcesses")

		typedParams, ok := params.(*papi.UnregisterProcessesArgs)
		c.Assert(ok, gc.Equals, true)

		c.Check(len(typedParams.IDs), gc.Equals, 1)
		c.Check(typedParams.UnitTag, gc.Equals, s.tag)

		return nil
	}

	client := pclient.NewProcessClient(s.stub, s.stub)
	err := client.UnregisterProcesses(s.tag, "idfoo")
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
