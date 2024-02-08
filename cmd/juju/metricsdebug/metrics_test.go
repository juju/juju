// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/metricsdebug"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type mockGetMetricsClient struct {
	testing.Stub
	metrics []params.MetricResult
}

func (m *mockGetMetricsClient) GetMetrics(tags ...string) ([]params.MetricResult, error) {
	m.AddCall("GetMetrics", tags)
	return m.metrics, m.NextErr()
}

func (m *mockGetMetricsClient) Close() error {
	m.AddCall("Close")
	return m.NextErr()
}

type metricsSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite

	client *mockGetMetricsClient
}

var _ = gc.Suite(&metricsSuite{})

func (s *metricsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.client = &mockGetMetricsClient{Stub: testing.Stub{}}
	s.PatchValue(metricsdebug.NewClient, func(_ modelcmd.ModelCommandBase) (metricsdebug.GetMetricsClient, error) {
		return s.client, nil
	})
}

func (s *metricsSuite) TestSort(c *gc.C) {
	s.client.metrics = []params.MetricResult{{
		Unit:  "unit-metered-0",
		Key:   "c-s",
		Value: "5.0",
		Time:  time.Date(2016, 8, 22, 12, 02, 04, 0, time.UTC),
	}, {
		Unit:  "unit-metered-0",
		Key:   "b-s",
		Value: "10.0",
		Time:  time.Date(2016, 8, 22, 12, 02, 04, 0, time.UTC),
	}, {
		Unit:  "unit-metered-0",
		Key:   "a-s",
		Value: "15.0",
		Time:  time.Date(2016, 8, 22, 12, 02, 04, 0, time.UTC),
	}, {
		Unit:   "unit-metered-0",
		Key:    "a-s",
		Value:  "5.0",
		Labels: map[string]string{"quux": "baz"},
		Time:   time.Date(2016, 8, 22, 12, 02, 04, 0, time.UTC),
	}, {
		Unit:   "unit-metered-0",
		Key:    "a-s",
		Value:  "10.0",
		Labels: map[string]string{"foo": "bar"},
		Time:   time.Date(2016, 8, 22, 12, 02, 04, 0, time.UTC),
	}}
	ctx, err := cmdtesting.RunCommand(c, metricsdebug.NewMetricsCommandForTest(), "metered/0")
	c.Assert(err, jc.ErrorIsNil)
	s.client.CheckCall(c, 0, "GetMetrics", []string{"unit-metered-0"})
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
UNIT          	           TIMESTAMP	METRIC	VALUE	  LABELS
unit-metered-0	2016-08-22T12:02:04Z	   a-s	 15.0	        
unit-metered-0	2016-08-22T12:02:04Z	   a-s	 10.0	 foo=bar
unit-metered-0	2016-08-22T12:02:04Z	   a-s	  5.0	quux=baz
unit-metered-0	2016-08-22T12:02:04Z	   b-s	 10.0	        
unit-metered-0	2016-08-22T12:02:04Z	   c-s	  5.0	        
`[1:])
}

func (s *metricsSuite) TestDefaultTabularFormat(c *gc.C) {
	s.client.metrics = []params.MetricResult{{
		Unit:  "unit-metered-0",
		Key:   "pongs",
		Value: "15.0",
		Time:  time.Date(2016, 8, 22, 12, 02, 04, 0, time.UTC),
	}, {
		Unit:  "unit-metered-0",
		Key:   "pings",
		Value: "5.0",
		Time:  time.Date(2016, 8, 22, 12, 02, 03, 0, time.UTC),
	}}
	ctx, err := cmdtesting.RunCommand(c, metricsdebug.NewMetricsCommandForTest(), "metered/0")
	c.Assert(err, jc.ErrorIsNil)
	s.client.CheckCall(c, 0, "GetMetrics", []string{"unit-metered-0"})
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `UNIT          	           TIMESTAMP	METRIC	VALUE	LABELS
unit-metered-0	2016-08-22T12:02:03Z	 pings	  5.0	      
unit-metered-0	2016-08-22T12:02:04Z	 pongs	 15.0	      
`)
}

func (s *metricsSuite) TestJSONFormat(c *gc.C) {
	s.client.metrics = []params.MetricResult{{
		Unit:  "unit-metered-0",
		Key:   "pings",
		Value: "5.0",
		Time:  time.Date(2016, 8, 22, 12, 02, 03, 0, time.UTC),
	}, {
		Unit:  "unit-metered-0",
		Key:   "pongs",
		Value: "15.0",
		Time:  time.Date(2016, 8, 22, 12, 02, 04, 0, time.UTC),
	}, {
		Unit:   "unit-metered-0",
		Key:    "pongs",
		Value:  "10.0",
		Time:   time.Date(2016, 8, 22, 12, 02, 04, 0, time.UTC),
		Labels: map[string]string{"foo": "bar"},
	}}
	ctx, err := cmdtesting.RunCommand(c, metricsdebug.NewMetricsCommandForTest(), "metered", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	s.client.CheckCall(c, 0, "GetMetrics", []string{"application-metered"})
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `[{"unit":"unit-metered-0","timestamp":"2016-08-22T12:02:03Z","metric":"pings","value":"5.0"},{"unit":"unit-metered-0","timestamp":"2016-08-22T12:02:04Z","metric":"pongs","value":"15.0"},{"unit":"unit-metered-0","timestamp":"2016-08-22T12:02:04Z","metric":"pongs","value":"10.0","labels":{"foo":"bar"}}]
`)
}

func (s *metricsSuite) TestYAMLFormat(c *gc.C) {
	s.client.metrics = []params.MetricResult{{
		Unit:  "unit-metered-0",
		Key:   "pings",
		Value: "5.0",
		Time:  time.Date(2016, 8, 22, 12, 02, 03, 0, time.UTC),
	}, {
		Unit:  "unit-metered-0",
		Key:   "pongs",
		Value: "15.0",
		Time:  time.Date(2016, 8, 22, 12, 02, 04, 0, time.UTC),
	}, {
		Unit:   "unit-metered-0",
		Key:    "pongs",
		Value:  "10.0",
		Time:   time.Date(2016, 8, 22, 12, 02, 04, 0, time.UTC),
		Labels: map[string]string{"foo": "bar"},
	}}
	ctx, err := cmdtesting.RunCommand(c, metricsdebug.NewMetricsCommandForTest(), "metered", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	s.client.CheckCall(c, 0, "GetMetrics", []string{"application-metered"})
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `- unit: unit-metered-0
  timestamp: 2016-08-22T12:02:03Z
  metric: pings
  value: "5.0"
- unit: unit-metered-0
  timestamp: 2016-08-22T12:02:04Z
  metric: pongs
  value: "15.0"
- unit: unit-metered-0
  timestamp: 2016-08-22T12:02:04Z
  metric: pongs
  value: "10.0"
  labels:
    foo: bar
`)
}

func (s *metricsSuite) TestAll(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, metricsdebug.NewMetricsCommandForTest(), "--all")
	c.Assert(err, jc.ErrorIsNil)
	s.client.CheckCall(c, 0, "GetMetrics", []string(nil))
}

func (s *metricsSuite) TestAllWithExtraArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, metricsdebug.NewMetricsCommandForTest(), "--all", "metered")
	c.Assert(err, gc.ErrorMatches, "cannot use --all with additional entities")
}

func (s *metricsSuite) TestInvalidUnitName(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, metricsdebug.NewMetricsCommandForTest(), "metered-/0")
	c.Assert(err, gc.ErrorMatches, `"metered-/0" is not a valid unit or application`)
}

func (s *metricsSuite) TestAPIClientError(c *gc.C) {
	s.client.SetErrors(errors.New("a silly error"))
	_, err := cmdtesting.RunCommand(c, metricsdebug.NewMetricsCommandForTest(), "metered/0")
	c.Assert(err, gc.ErrorMatches, `a silly error`)
}

func (s *metricsSuite) TestNoArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, metricsdebug.NewMetricsCommandForTest())
	c.Assert(err, gc.ErrorMatches, "you need to specify at least one unit or application")
}
