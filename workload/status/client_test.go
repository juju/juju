// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"encoding/json"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api"
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
	in := []api.Workload{
		{
			Definition: api.WorkloadDefinition{
				Name:        "foo",
				Description: "desc",
				Type:        "type",
				TypeOptions: map[string]string{"foo": "bar"},
				Command:     "command",
				Image:       "image",
				Ports:       []api.WorkloadPort{{External: 1, Internal: 2, Endpoint: "endpoint"}},
				Volumes:     []api.WorkloadVolume{{ExternalMount: "ext", InternalMount: "int", Mode: "rw", Name: "foo"}},
				EnvVars:     map[string]string{"baz": "baz"},
			},
			Status: api.WorkloadStatus{
				State:   workload.StateRunning,
				Message: "okay",
			},
			Details: api.WorkloadDetails{
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
				State:       workload.StateRunning,
				Info:        "okay",
				PluginState: in[0].Details.Status.State,
			},
		},
	}

	c.Assert(s, gc.DeepEquals, expected)
}
