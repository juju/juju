package client_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api/internal"
	"github.com/juju/juju/workload/api/internal/client"
)

type clientSuite struct {
	stub       *testing.Stub
	facade     *stubFacade
	tag        string
	workload   internal.Workload
	definition internal.WorkloadDefinition
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.stub = &testing.Stub{}
	s.facade = &stubFacade{stub: s.stub}
	s.tag = "machine-tag"
	s.definition = internal.WorkloadDefinition{
		Name:        "foobar",
		Description: "desc",
		Type:        "type",
		TypeOptions: map[string]string{"foo": "bar"},
		Command:     "cmd",
		Image:       "img",
		Ports: []internal.WorkloadPort{{
			External: 8080,
			Internal: 80,
			Endpoint: "endpoint",
		}},
		Volumes: []internal.WorkloadVolume{{
			ExternalMount: "/foo/bar",
			InternalMount: "/baz/bat",
			Mode:          "ro",
			Name:          "volname",
		}},
		EnvVars: map[string]string{"envfoo": "bar"},
	}

	s.workload = internal.Workload{
		Definition: s.definition,
		Status: internal.WorkloadStatus{
			State:   workload.StateRunning,
			Message: "okay",
		},
		Details: internal.WorkloadDetails{
			ID: "idfoo",
			Status: internal.PluginStatus{
				State: "workload status",
			},
		},
	}

}

func (s *clientSuite) TestTrack(c *gc.C) {
	numStubCalls := 0
	s.facade.FacadeCallFn = func(name string, params, response interface{}) error {
		numStubCalls++
		c.Check(name, gc.Equals, "Track")

		typedResponse, ok := response.(*internal.WorkloadResults)
		c.Assert(ok, gc.Equals, true)

		typedResponse.Results = append(typedResponse.Results, internal.WorkloadResult{
			ID: internal.FullID{
				Class: "idfoo",
				ID:    "bar",
			},
			Error: nil,
		})

		return nil
	}

	pclient := client.NewHookContextClient(s.facade)

	workloadInfo := internal.API2Workload(s.workload)
	ids, err := pclient.Track(workloadInfo)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(len(ids), gc.Equals, 1)
	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(ids[0], gc.Equals, "idfoo/bar")
}

func (s *clientSuite) TestList(c *gc.C) {
	numStubCalls := 0

	s.facade.FacadeCallFn = func(name string, params, response interface{}) error {
		numStubCalls++
		c.Check(name, gc.Equals, "List")

		typedResponse, ok := response.(*internal.ListResults)
		c.Assert(ok, gc.Equals, true)

		result := internal.ListResult{
			ID: internal.FullID{
				Class: s.workload.Definition.Name,
				ID:    s.workload.Details.ID,
			},
			Info:  s.workload,
			Error: nil,
		}
		typedResponse.Results = append(typedResponse.Results, result)

		return nil
	}
	pclient := client.NewHookContextClient(s.facade)

	workloads, err := pclient.List(s.tag)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(len(workloads), gc.Equals, 1)
	c.Check(numStubCalls, gc.Equals, 1)

	wl := internal.API2Workload(s.workload)
	c.Check(workloads[0], gc.DeepEquals, wl)
}

func (s *clientSuite) TestSetStatus(c *gc.C) {
	numStubCalls := 0

	s.facade.FacadeCallFn = func(name string, params, response interface{}) error {
		numStubCalls++
		c.Check(name, gc.Equals, "SetStatus")

		typedParams, ok := params.(*internal.SetStatusArgs)
		c.Assert(ok, gc.Equals, true)

		c.Check(len(typedParams.Args), gc.Equals, 1)

		arg := typedParams.Args[0]
		c.Check(arg, jc.DeepEquals, internal.SetStatusArg{
			ID: internal.FullID{
				Class: "idfoo",
				ID:    "bar",
			},
			Status: workload.StateRunning,
		})

		return nil
	}

	pclient := client.NewHookContextClient(s.facade)
	_, err := pclient.SetStatus(workload.StateRunning, "idfoo/bar")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(numStubCalls, gc.Equals, 1)
}

func (s *clientSuite) TestUntrack(c *gc.C) {
	numStubCalls := 0

	s.facade.FacadeCallFn = func(name string, params, response interface{}) error {
		numStubCalls++
		c.Check(name, gc.Equals, "Untrack")

		typedParams, ok := params.(*internal.UntrackArgs)
		c.Assert(ok, gc.Equals, true)

		c.Check(len(typedParams.IDs), gc.Equals, 1)

		return nil
	}

	pclient := client.NewHookContextClient(s.facade)
	_, err := pclient.Untrack(s.tag)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(numStubCalls, gc.Equals, 1)
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
