// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"

	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"

	cmdutil "github.com/juju/juju/cmd/jujud/util"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/metrics/spool"
)

var _ = gc.Suite(&collectMetricsTestSuite{})

type collectMetricsTestSuite struct {
	jujutesting.JujuConnSuite
	dataDir string
	cleanup func()
}

func (s *collectMetricsTestSuite) SetUpTest(c *gc.C) {
	s.dataDir = c.MkDir()
	s.cleanup = jtesting.PatchValue(&cmdutil.DataDir, s.dataDir)
}

func (s *collectMetricsTestSuite) TearDownTest(c *gc.C) {
	s.cleanup()
}

func (s *collectMetricsTestSuite) TestArgParsing(c *gc.C) {
	for i, test := range []struct {
		about     string
		args      []string
		expectErr string
		unit      string
	}{{
		about:     "no args",
		expectErr: "missing arguments",
	}, {
		about:     "too many args",
		args:      []string{"wordpress/0", "unknown"},
		expectErr: `unrecognized args: \["unknown"\]`,
	}, {
		about:     "wrong unit format",
		args:      []string{"wordpres_0", "cs:~charmers/wordpress-0"},
		expectErr: "\"wordpres_0\" is not a valid tag",
	}, {
		about: "all is well",
		args:  []string{"wordpress/0"},
		unit:  "unit-wordpress-0",
	},
	} {
		c.Logf("running test %d: %s", i, test.about)
		collectCommand := &CollectMetricsCommand{}
		err := testing.InitCommand(collectCommand, test.args)
		if test.expectErr != "" {
			c.Assert(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(collectCommand.unit.String(), gc.Equals, test.unit)
		}
	}
}

func (s *collectMetricsTestSuite) TestCollectMetrics(c *gc.C) {
	ft.Entries{
		ft.Dir{"agents", 0755},
		ft.Dir{"agents/unit-test-charm-1", 0755},
		ft.Dir{"agents/unit-test-charm-1/charm", 0755},
		ft.File{"agents/unit-test-charm-1/charm/metadata.yaml", `
name: test-charm
summary: "A test charm"
description: ""
`, 0644},
		ft.Dir{"agents/unit-test-charm-1/charm/hooks", 0755},
		ft.File{"agents/unit-test-charm-1/charm/hooks/collect-metrics", `#!/bin/bash -e

exit 0
`, 0777},
	}.Create(c, s.dataDir)

	tests := []struct {
		about       string
		charmURL    string
		metrics     string
		expectError string
	}{{
		about:    "local charm",
		charmURL: "local:trusty/test-charm",
		metrics: `
metrics:
  pings:
    type: gauge
    description: Description of the metric.
  juju-units:
`,
	}, {
		about:    "cs charm",
		charmURL: "cs:trusty/test-charm",
		metrics: `
metrics:
  pings:
    type: gauge
    description: Description of the metric.
  juju-units:
`,
		expectError: "not a local charm",
	}, {
		about:       "local charm, but not metered",
		charmURL:    "local:trusty/test-charm",
		expectError: "not a metered charm",
	},
	}

	for i, test := range tests {
		c.Logf("running test %d: %v", i, test.about)
		ft.Entries{
			ft.File{"agents/unit-test-charm-1/charm/metrics.yaml", test.metrics, 0644},
			ft.File{"agents/unit-test-charm-1/charm/.juju-charm", test.charmURL, 0644},
		}.Create(c, s.dataDir)
		ctx, err := testing.RunCommand(c, &CollectMetricsCommand{}, "test-charm/1")
		if test.expectError == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(testing.Stdout(ctx), gc.Not(gc.Equals), "")
			var batches []spool.MetricBatch
			err = json.Unmarshal([]byte(testing.Stdout(ctx)), &batches)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(batches, gc.HasLen, 1)
			c.Assert(batches[0].CharmURL, gc.Equals, "local:trusty/test-charm")
			c.Assert(batches[0].UnitTag, gc.Equals, "unit-test-charm-1")
			c.Assert(batches[0].Metrics, gc.HasLen, 1)
			c.Assert(batches[0].Metrics[0].Key, gc.Equals, "juju-units")
			c.Assert(batches[0].Metrics[0].Value, gc.Equals, "1")
		} else {
			c.Assert(err, gc.ErrorMatches, test.expectError)
		}
	}
}
