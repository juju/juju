// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner"
)

type MetricsRecorderSuite struct {
	testing.IsolationSuite

	paths RealPaths
}

var _ = gc.Suite(&MetricsRecorderSuite{})

func (s *MetricsRecorderSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.paths = NewRealPaths(c)
	s.PatchValue(&runner.LockTimeout, jujutesting.ShortWait)
}

func (s *MetricsRecorderSuite) TestMetricRecorderInit(c *gc.C) {
	w, err := runner.NewJSONMetricsRecorder(s.paths.GetMetricsSpoolDir(), "local:precise/wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	err = w.AddMetric("pings", "5", time.Now())
	c.Assert(err, jc.ErrorIsNil)
	err = w.Close()
	c.Assert(err, jc.ErrorIsNil)

	r, err := runner.NewJSONMetricsReader(s.paths.GetMetricsSpoolDir())
	c.Assert(err, jc.ErrorIsNil)
	batches, err := r.Open()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 1)
	batch := batches[0]
	c.Assert(batch.CharmURL, gc.Equals, "local:precise/wordpress")
	c.Assert(batch.UUID, gc.Not(gc.Equals), "")
	c.Assert(batch.Metrics, gc.HasLen, 1)
	c.Assert(batch.Metrics[0].Key, gc.Equals, "pings")
	c.Assert(batch.Metrics[0].Value, gc.Equals, "5")

	err = r.Close()
	c.Assert(err, jc.ErrorIsNil)
}

type MetricsReaderSuite struct {
	paths RealPaths

	w runner.MetricsRecorder
}

var _ = gc.Suite(&MetricsReaderSuite{})

func (s *MetricsReaderSuite) SetUpTest(c *gc.C) {
	s.paths = NewRealPaths(c)

	var err error
	s.w, err = runner.NewJSONMetricsRecorder(s.paths.GetMetricsSpoolDir(), "local:precise/wordpress")
	c.Assert(err, jc.ErrorIsNil)
	err = s.w.AddMetric("pings", "5", time.Now())
	c.Assert(err, jc.ErrorIsNil)
	err = s.w.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MetricsReaderSuite) TestTwoSimultaneousReaders(c *gc.C) {
	r, err := runner.NewJSONMetricsReader(s.paths.GetMetricsSpoolDir())
	c.Assert(err, jc.ErrorIsNil)

	r2, err := runner.NewJSONMetricsReader(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r2, gc.NotNil)
	err = r2.Close()
	c.Assert(err, jc.ErrorIsNil)
	err = r.Close()
	c.Assert(err, jc.ErrorIsNil)

}

func (s *MetricsReaderSuite) TestBlockedReaders(c *gc.C) {
	r, err := runner.NewJSONMetricsReader(s.paths.GetMetricsSpoolDir())
	c.Assert(err, jc.ErrorIsNil)
	_, err = r.Open()
	c.Assert(err, jc.ErrorIsNil)

	r2, err := runner.NewJSONMetricsReader(s.paths.GetMetricsSpoolDir())
	c.Assert(err, jc.ErrorIsNil)
	_, err = r2.Open()
	c.Assert(err, gc.ErrorMatches, `lock timeout exceeded`)
	err = r.Close()
	c.Assert(err, jc.ErrorIsNil)

}

func (s *MetricsReaderSuite) TestUnblockedReaders(c *gc.C) {
	r, err := runner.NewJSONMetricsReader(s.paths.GetMetricsSpoolDir())
	c.Assert(err, jc.ErrorIsNil)
	err = r.Close()
	c.Assert(err, jc.ErrorIsNil)

	r2, err := runner.NewJSONMetricsReader(s.paths.GetMetricsSpoolDir())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r2, gc.NotNil)
	err = r2.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MetricsReaderSuite) TestRemoval(c *gc.C) {
	r, err := runner.NewJSONMetricsReader(s.paths.GetMetricsSpoolDir())
	c.Assert(err, jc.ErrorIsNil)

	batches, err := r.Open()
	c.Assert(err, jc.ErrorIsNil)
	for _, batch := range batches {
		err := r.Remove(batch.UUID)
		c.Assert(err, jc.ErrorIsNil)
	}
	err = r.Close()
	c.Assert(err, jc.ErrorIsNil)

	batches, err = r.Open()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 0)
	err = r.Close()
	c.Assert(err, jc.ErrorIsNil)

}
