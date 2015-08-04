// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"encoding/json"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
)

type clientSuite struct{}

var _ = gc.Suite(&clientSuite{})

func (*clientSuite) TestWrongObj(c *gc.C) {
	out := Format([]byte("foo"))
	if _, ok := out.(error); !ok {
		c.Errorf("Expected error, got %#v", out)
	}
}

func (*clientSuite) TestGood(c *gc.C) {
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
			Status: api.ProcessStatus{
				State:   process.StateRunning,
				Message: "okay",
			},
			Details: api.ProcessDetails{
				ID: "id",
				Status: api.PluginStatus{
					State: "Running",
				},
			},
		},
	}

	b, err := json.Marshal(in)
	c.Assert(err, jc.ErrorIsNil)
	out := Format(b)
	s, ok := out.(map[string]cliDetails)
	if !ok {
		c.Fatalf("Expected map[string]cliDetails but got %#v", out)
	}
	expected := map[string]cliDetails{
		in[0].Definition.Name: cliDetails{
			ID:   in[0].Details.ID,
			Type: in[0].Definition.Type,
			Status: cliStatus{
				State:       process.StateRunning,
				Info:        "okay",
				PluginState: in[0].Details.Status.State,
			},
		},
	}

	c.Assert(s, gc.DeepEquals, expected)
}
