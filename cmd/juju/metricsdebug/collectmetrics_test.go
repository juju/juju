// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	"github.com/juju/charm/v7"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
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

	actionTag1 = names.NewActionTag("01234567-89ab-cdef-0123-456789abcdef")
	actionTag2 = names.NewActionTag("11234567-89ab-cdef-0123-456789abcdef")

	tests = []struct {
		about     string
		args      []string
		stdout    string
		results   [][]params.ActionResult
		actionMap map[string]params.ActionResult
		err       string
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
		results: [][]params.ActionResult{
			{{
				Action: &params.Action{
					Tag: actionTag1.String(),
				},
			}},
			{{
				Action: &params.Action{
					Tag: actionTag2.String(),
				},
			}},
		},
		actionMap: map[string]params.ActionResult{
			actionTag1.Id(): {
				Action: &params.Action{
					Tag:      actionTag1.String(),
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"Stdout": "ok",
					"Stderr": "",
				},
			},
			actionTag2.Id(): {
				Action: &params.Action{
					Tag: actionTag2.String(),
				},
				Output: map[string]interface{}{
					"Stdout": "ok",
					"Stderr": "",
				},
			},
		},
	}, {
		about: "invalid tag returned",
		args:  []string{"uptime"},
		results: [][]params.ActionResult{
			{{
				Action: &params.Action{
					Tag: "invalid",
				},
			}},
		},
		stdout: `failed to collect metrics: "invalid" is not a valid tag\n`,
	}, {
		about: "no action found",
		args:  []string{"uptime"},
		results: [][]params.ActionResult{
			{{
				Action: &params.Action{
					Tag: actionTag1.String(),
				},
			}},
		},
		stdout: "failed to collect metrics: plm\n",
	}, {
		about: "fail to parse result",
		args:  []string{"uptime"},
		results: [][]params.ActionResult{
			{{
				Action: &params.Action{
					Tag: actionTag1.String(),
				},
			}},
		},
		actionMap: map[string]params.ActionResult{
			actionTag1.Id(): {},
		},
		stdout: "failed to collect metrics: could not read stdout\n",
	}, {
		about: "no results on sendResults",
		args:  []string{"uptime"},
		results: [][]params.ActionResult{
			{{
				Action: &params.Action{
					Tag: actionTag1.String(),
				},
			}},
		},
		actionMap: map[string]params.ActionResult{
			actionTag1.Id(): {
				Action: &params.Action{
					Tag:      actionTag2.String(),
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"Stdout": "ok",
					"Stderr": "",
				},
			},
		},
		stdout: "failed to send metrics for unit uptime/0: no results\n",
	}, {
		about: "too many sendResults",
		args:  []string{"uptime"},
		results: [][]params.ActionResult{
			{{
				Action: &params.Action{
					Tag: actionTag1.String(),
				},
			}},
			{{
				Action: &params.Action{
					Tag: actionTag1.String(),
				},
			}, {
				Action: &params.Action{
					Tag: actionTag2.String(),
				},
			}},
		},
		actionMap: map[string]params.ActionResult{
			actionTag1.Id(): {
				Action: &params.Action{
					Tag:      actionTag2.String(),
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"Stdout": "ok",
					"Stderr": "",
				},
			},
		},
		stdout: "failed to send metrics for unit uptime/0\n",
	}, {
		about: "sendResults error",
		args:  []string{"uptime"},
		results: [][]params.ActionResult{
			{{
				Action: &params.Action{
					Tag: actionTag1.String(),
				},
			}},
			{{
				Error: &params.Error{
					Message: "permission denied",
				},
			}},
		},
		actionMap: map[string]params.ActionResult{
			actionTag1.Id(): {
				Action: &params.Action{
					Tag:      actionTag2.String(),
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"Stdout": "ok",
					"Stderr": "",
				},
			},
		},
		stdout: "failed to send metrics for unit uptime/0: permission denied\n",
	}, {
		about: "couldn't get sendResults action",
		args:  []string{"uptime"},
		results: [][]params.ActionResult{
			{{
				Action: &params.Action{
					Tag: actionTag1.String(),
				},
			}},
			{{
				Action: &params.Action{
					Tag: actionTag2.String(),
				},
			}},
		},
		actionMap: map[string]params.ActionResult{
			actionTag1.Id(): {
				Action: &params.Action{
					Tag:      actionTag2.String(),
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"Stdout": "ok",
					"Stderr": "",
				},
			},
		},
		stdout: "failed to send metrics for unit uptime/0: plm\n",
	}, {
		about: "couldn't parse sendResults action",
		args:  []string{"uptime"},
		results: [][]params.ActionResult{
			{{
				Action: &params.Action{
					Tag: actionTag1.String(),
				},
			}},
			{{
				Action: &params.Action{
					Tag: actionTag2.String(),
				},
			}},
		},
		actionMap: map[string]params.ActionResult{
			actionTag1.Id(): {
				Action: &params.Action{
					Tag:      actionTag2.String(),
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"Stdout": "ok",
					"Stderr": "",
				},
			},
			actionTag2.Id(): {},
		},
		stdout: "failed to send metrics for unit uptime/0: could not read stdout\n",
	}, {
		about: "sendResults action stderr",
		args:  []string{"uptime"},
		results: [][]params.ActionResult{
			{{
				Action: &params.Action{
					Tag: actionTag1.String(),
				},
			}},
			{{
				Action: &params.Action{
					Tag: actionTag2.String(),
				},
			}},
		},
		actionMap: map[string]params.ActionResult{
			actionTag1.Id(): {
				Action: &params.Action{
					Tag:      actionTag2.String(),
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"Stdout": "ok",
					"Stderr": "",
				},
			},
			actionTag2.Id(): {
				Action: &params.Action{
					Tag:      actionTag2.String(),
					Receiver: "unit-uptime-0",
				},
				Output: map[string]interface{}{
					"Stdout": "garbage",
					"Stderr": "kek",
				},
			},
		},
		stdout: "failed to send metrics for unit uptime/0: kek\n",
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

	results [][]params.ActionResult
	err     string
}

// Run implements the runClient interface.
func (t *testRunClient) Run(run params.RunParams) ([]params.ActionResult, error) {
	t.AddCall("Run", run)
	if t.err != "" {
		return nil, errors.New(t.err)
	}
	if len(t.results) == 0 {
		return nil, errors.New("no results")
	}
	r := t.results[0]
	t.results = t.results[1:]
	return r, nil
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

func (t *testApplicationClient) GetCharmURL(service string) (*charm.URL, error) {
	url := charm.MustParseURL(t.charmURL)
	return url, t.NextErr()
}

func (t *testApplicationClient) Close() error {
	return t.NextErr()
}

func noConn(_ modelcmd.ModelCommandBase) (api.Connection, error) {
	return nil, nil
}
