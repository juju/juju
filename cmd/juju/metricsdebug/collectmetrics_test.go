// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	"github.com/juju/charm/v13"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	actionapi "github.com/juju/juju/api/client/action"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/juju/metricsdebug"
	"github.com/juju/juju/cmd/modelcmd"
	coretesting "github.com/juju/juju/testing"
)

type collectMetricsSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

var (
	_ = gc.Suite(&collectMetricsSuite{})

	actionID1 = "01234567-89ab-cdef-0123-456789abcdef"
	actionID2 = "11234567-89ab-cdef-0123-456789abcdef"

	tests = []struct {
		about          string
		args           []string
		stdout, stderr string
		results        [][]actionapi.ActionResult
		actionMap      map[string]actionapi.ActionResult
		err            string
	}{{
		about: "missing args",
		err:   "you need to specify a unit or application.",
	}, {
		about: "invalid application name",
		args:  []string{"application_1-0"},
		err:   `"application_1-0" is not a valid unit or application`,
	}, {
		about: "all is well",
		args:  []string{"uptime"},
		results: [][]actionapi.ActionResult{
			{{
				Action: &actionapi.Action{
					ID: actionID1,
				},
			}},
			{{
				Action: &actionapi.Action{
					ID: actionID2,
				},
			}},
		},
		actionMap: map[string]actionapi.ActionResult{
			actionID1: {
				Action: &actionapi.Action{
					ID:       actionID1,
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"stdout": "ok",
					"stderr": "",
				},
			},
			actionID2: {
				Action: &actionapi.Action{
					ID: actionID2,
				},
				Output: map[string]interface{}{
					"stdout": "ok",
					"stderr": "",
				},
			},
		},
	}, {
		about: "no action found",
		args:  []string{"uptime"},
		results: [][]actionapi.ActionResult{
			{{
				Action: &actionapi.Action{
					ID: actionID1,
				},
			}},
		},
		stderr: "failed to collect metrics: plm\n",
	}, {
		about: "fail to parse result",
		args:  []string{"uptime"},
		results: [][]actionapi.ActionResult{
			{{
				Action: &actionapi.Action{
					ID: actionID1,
				},
			}},
		},
		actionMap: map[string]actionapi.ActionResult{
			actionID1: {},
		},
		stderr: "failed to collect metrics: could not read stdout\n",
	}, {
		about: "no results on sendResults",
		args:  []string{"uptime"},
		results: [][]actionapi.ActionResult{
			{{
				Action: &actionapi.Action{
					ID: actionID1,
				},
			}},
		},
		actionMap: map[string]actionapi.ActionResult{
			actionID1: {
				Action: &actionapi.Action{
					ID:       actionID2,
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"stdout": "ok",
					"stderr": "",
				},
			},
		},
		stderr: "failed to send metrics for unit uptime/0: no results\n",
	}, {
		about: "too many sendResults",
		args:  []string{"uptime"},
		results: [][]actionapi.ActionResult{
			{{
				Action: &actionapi.Action{
					ID: actionID1,
				},
			}},
			{{
				Action: &actionapi.Action{
					ID: actionID1,
				},
			}, {
				Action: &actionapi.Action{
					ID: actionID2,
				},
			}},
		},
		actionMap: map[string]actionapi.ActionResult{
			actionID1: {
				Action: &actionapi.Action{
					ID:       actionID2,
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"stdout": "ok",
					"stderr": "",
				},
			},
		},
		stderr: "failed to send metrics for unit uptime/0\n",
	}, {
		about: "sendResults error",
		args:  []string{"uptime"},
		results: [][]actionapi.ActionResult{
			{{
				Action: &actionapi.Action{
					ID: actionID1,
				},
			}},
			{{
				Error: errors.New("permission denied"),
			}},
		},
		actionMap: map[string]actionapi.ActionResult{
			actionID1: {
				Action: &actionapi.Action{
					ID:       actionID2,
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"stdout": "ok",
					"stderr": "",
				},
			},
		},
		stderr: "failed to send metrics for unit uptime/0: permission denied\n",
	}, {
		about: "couldn't get sendResults action",
		args:  []string{"uptime"},
		results: [][]actionapi.ActionResult{
			{{
				Action: &actionapi.Action{
					ID: actionID1,
				},
			}},
			{{
				Action: &actionapi.Action{
					ID: actionID2,
				},
			}},
		},
		actionMap: map[string]actionapi.ActionResult{
			actionID1: {
				Action: &actionapi.Action{
					ID:       actionID2,
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"stdout": "ok",
					"stderr": "",
				},
			},
		},
		stderr: "failed to send metrics for unit uptime/0: plm\n",
	}, {
		about: "couldn't parse sendResults action",
		args:  []string{"uptime"},
		results: [][]actionapi.ActionResult{
			{{
				Action: &actionapi.Action{
					ID: actionID1,
				},
			}},
			{{
				Action: &actionapi.Action{
					ID: actionID2,
				},
			}},
		},
		actionMap: map[string]actionapi.ActionResult{
			actionID1: {
				Action: &actionapi.Action{
					ID:       actionID2,
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"stdout": "ok",
					"stderr": "",
				},
			},
			actionID2: {},
		},
		stderr: "failed to send metrics for unit uptime/0: could not read stdout\n",
	}, {
		about: "sendResults action stderr",
		args:  []string{"uptime"},
		results: [][]actionapi.ActionResult{
			{{
				Action: &actionapi.Action{
					ID: actionID1,
				},
			}},
			{{
				Action: &actionapi.Action{
					ID: actionID2,
				},
			}},
		},
		actionMap: map[string]actionapi.ActionResult{
			actionID1: {
				Action: &actionapi.Action{
					ID:       actionID2,
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"stdout": "ok",
					"stderr": "",
				},
			},
			actionID2: {
				Action: &actionapi.Action{
					ID:       actionID2,
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"stdout": "garbage",
					"stderr": "kek",
				},
			},
		},
		stderr: "failed to send metrics for unit uptime/0: kek\n",
	}}
)

func (s *collectMetricsSuite) TestCollectMetricsLocal(c *gc.C) {
	runClient := &testRunClient{}
	applicationClient := &testApplicationClient{}
	applicationClient.charmURL = "local:quantal/charm"
	s.PatchValue(metricsdebug.NewAPIConn, noConn)
	s.PatchValue(metricsdebug.NewRunClient, metricsdebug.NewRunClientFnc(runClient))

	for i, test := range tests {
		c.Logf("running test %d: %v", i, test.about)
		runClient.reset()
		if test.results != nil {
			runClient.results = test.results
		}
		metricsdebug.PatchGetActionResult(s.PatchValue, test.actionMap)
		ctx, err := cmdtesting.RunCommand(c, metricsdebug.NewCollectMetricsCommandForTest(), test.args...)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(cmdtesting.Stdout(ctx), gc.Matches, test.stdout)
			c.Assert(cmdtesting.Stderr(ctx), gc.Matches, test.stderr)
		}
	}
}

func (s *collectMetricsSuite) TestCollectMetricsRemote(c *gc.C) {
	runClient := &testRunClient{}
	applicationClient := &testApplicationClient{}
	applicationClient.charmURL = "quantal/charm"
	s.PatchValue(metricsdebug.NewAPIConn, noConn)
	s.PatchValue(metricsdebug.NewRunClient, metricsdebug.NewRunClientFnc(runClient))

	for i, test := range tests {
		c.Logf("running test %d: %v", i, test.about)
		runClient.reset()
		if test.results != nil {
			runClient.results = test.results
		}
		metricsdebug.PatchGetActionResult(s.PatchValue, test.actionMap)
		ctx, err := cmdtesting.RunCommand(c, metricsdebug.NewCollectMetricsCommandForTest(), test.args...)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(cmdtesting.Stdout(ctx), gc.Matches, test.stdout)
		}
	}
}

type testRunClient struct {
	action.APIClient
	testing.Stub

	results [][]actionapi.ActionResult
	err     string
}

// Run implements the runClient interface.
func (t *testRunClient) Run(run actionapi.RunParams) (actionapi.EnqueuedActions, error) {
	t.AddCall("Run", run)
	if t.err != "" {
		return actionapi.EnqueuedActions{}, errors.New(t.err)
	}
	if len(t.results) == 0 {
		return actionapi.EnqueuedActions{}, errors.New("no results")
	}
	r := t.results[0]
	t.results = t.results[1:]
	return actionapi.EnqueuedActions{
		OperationID: "1",
		Actions:     r,
	}, nil
}

// Close implements the runClient interface.
func (t *testRunClient) Close() error {
	t.AddCall("Close")
	return nil
}

func (t *testRunClient) reset() {
	t.ResetCalls()
	t.results = nil
	t.err = ""
}

type testApplicationClient struct {
	testing.Stub
	charmURL string
}

func (t *testApplicationClient) GetCharmURL(_ string) (*charm.URL, error) {
	url := charm.MustParseURL(t.charmURL)
	return url, t.NextErr()
}

func (t *testApplicationClient) Close() error {
	return t.NextErr()
}

func noConn(_ modelcmd.ModelCommandBase) (api.Connection, error) {
	return nil, nil
}
