package client

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process/api"
)

type statusSuite struct{}

var _ = gc.Suite(&statusSuite{})

func (*statusSuite) TestNil(c *gc.C) {
	c.Assert(ConvertAPItoCLI(nil), gc.IsNil)
}

func (*statusSuite) TestWrongObj(c *gc.C) {
	out := ConvertAPItoCLI("foo")
	if _, ok := out.(error); !ok {
		c.Errorf("Expected error, got %#v", out)
	}
}

func (*statusSuite) TestEmpty(c *gc.C) {
	out := ConvertAPItoCLI([]interface{}{})
	s, ok := out.(map[string]cliDetails)
	if !ok {
		c.Fatalf("Expected map[string]cliDetails but got %#v", out)
	}
	c.Assert(s, gc.HasLen, 0)
}

func (*statusSuite) TestGood(c *gc.C) {
	in := []api.Process{
		{
			Definition: api.ProcessDefinition{
				Name:        "foo",
				Description: "desc",
				Type:        "type",
				TypeOptions: map[string]string{"foo": "bar"},
				Command:     "command",
				Image:       "image",
				Ports:       []api.ProcessPort{{External: 1, Internal: 2, Endpoint: "endpoint"}},
				Volumes:     []api.ProcessVolume{{ExternalMount: "ext", InternalMount: "int", Mode: "rw", Name: "foo"}},
				EnvVars:     map[string]string{"baz": "baz"},
			},
			Details: api.ProcessDetails{
				ID: "id",
				Status: api.ProcessStatus{
					Label: "Running",
				},
			},
		},
	}

	out := ConvertAPItoCLI(in)
	s, ok := out.(map[string]cliDetails)
	if !ok {
		c.Fatalf("Expected map[string]cliDetails but got %#v", out)
	}
	expected := map[string]cliDetails{
		in[0].Definition.Name: cliDetails{
			ID:   in[0].Details.ID,
			Type: in[0].Definition.Type,
			Status: cliStatus{
				State: in[0].Details.Status.Label,
			},
		},
	}

	c.Assert(s, gc.DeepEquals, expected)
}
