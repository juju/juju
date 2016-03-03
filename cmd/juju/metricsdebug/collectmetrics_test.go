// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/metricsdebug"
	coretesting "github.com/juju/juju/testing"
)

type collectMetricsSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&collectMetricsSuite{})

func (s *collectMetricsSuite) TestCollectMetrics(c *gc.C) {
	runClient := &testRunClient{}
	cleanup := testing.PatchValue(metricsdebug.NewRunClient, metricsdebug.NewRunClientFnc(runClient))
	defer cleanup()

	tests := []struct {
		about   string
		args    []string
		stdout  string
		results [][]params.RunResult
		err     string
	}{{
		about: "missing args",
		err:   "you need to specify a unit or service.",
	}, {
		about: "invalid service name",
		args:  []string{"service_1-0"},
		err:   `"service_1-0" is not a valid unit or service`,
	}, {
		about: "all is well",
		args:  []string{"uptime"},
		results: [][]params.RunResult{
			[]params.RunResult{{
				UnitId: "uptime/0",
				ExecResponse: exec.ExecResponse{
					Stdout: []byte("ok"),
				},
			}},
			[]params.RunResult{{
				UnitId: "uptime/0",
				ExecResponse: exec.ExecResponse{
					Stdout: []byte("ok"),
				},
			}},
		},
	}, {
		about: "fail to collect metrics",
		args:  []string{"wordpress"},
		results: [][]params.RunResult{
			[]params.RunResult{{
				UnitId: "wordpress/0",
				ExecResponse: exec.ExecResponse{
					Stderr: []byte("nc: unix connect failed: No such file or directory"),
				},
			}},
		},
		stdout: "failed to collect metrics for unit wordpress/0: not a metered charm\n",
	}, {
		about: "fail to send metrics",
		args:  []string{"uptime"},
		results: [][]params.RunResult{
			[]params.RunResult{{
				UnitId: "uptime/0",
				ExecResponse: exec.ExecResponse{
					Stdout: []byte("ok"),
				},
			}},
			[]params.RunResult{{
				UnitId: "uptime/0",
				ExecResponse: exec.ExecResponse{
					Stderr: []byte("an embarrassing error"),
				},
			}},
		},
		stdout: "failed to send metrics for unit uptime/0: an embarrassing error\n",
	},
	}

	for i, test := range tests {
		c.Logf("running test %d: %v", i, test.about)
		runClient.reset()
		if test.results != nil {
			runClient.results = test.results
		}
		ctx, err := coretesting.RunCommand(c, metricsdebug.NewCollectMetricsCommand(), test.args...)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(coretesting.Stdout(ctx), gc.Matches, test.stdout)
		}
	}
}

type testRunClient struct {
	testing.Stub

	results [][]params.RunResult
	err     string
}

// Run implements the runClient interface.
func (t *testRunClient) Run(run params.RunParams) ([]params.RunResult, error) {
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
