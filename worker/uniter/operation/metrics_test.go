// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"errors"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/metrics"
	"github.com/juju/juju/worker/uniter/operation"
)

var _ = gc.Suite(&MetricsOperationSuite{})

type MetricsOperationSuite struct {
	spoolDir string
}

func (s *MetricsOperationSuite) SetUpTest(c *gc.C) {
	s.spoolDir = c.MkDir()

	declaredMetrics := map[string]corecharm.Metric{
		"pings": corecharm.Metric{Description: "test pings", Type: corecharm.MetricTypeAbsolute},
	}
	recorder, err := metrics.NewJSONMetricRecorder(s.spoolDir, declaredMetrics, "local:trusty/testcharm")
	c.Assert(err, jc.ErrorIsNil)

	err = recorder.AddMetric("pings", "50", time.Now())
	c.Assert(err, jc.ErrorIsNil)

	err = recorder.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MetricsOperationSuite) TestMetricSendingSuccess(c *gc.C) {
	apiSender := newTestAPIMetricSender()

	factory := operation.NewFactory(operation.FactoryParams{
		MetricSender:   apiSender,
		MetricSpoolDir: s.spoolDir,
	})

	sendOperation, err := factory.NewSendMetrics()
	c.Assert(err, gc.IsNil)

	_, err = sendOperation.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	_, err = sendOperation.Execute(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(apiSender.batches, gc.HasLen, 1)

	reader, err := metrics.NewJSONMetricReader(s.spoolDir)
	c.Assert(err, gc.IsNil)
	batches, err := reader.Read()
	c.Assert(err, gc.IsNil)
	c.Assert(batches, gc.HasLen, 0)
}

func (s *MetricsOperationSuite) TestSendingGetDuplicate(c *gc.C) {
	apiSender := newTestAPIMetricSender()

	factory := operation.NewFactory(operation.FactoryParams{
		MetricSender:   apiSender,
		MetricSpoolDir: s.spoolDir,
	})

	sendOperation, err := factory.NewSendMetrics()
	c.Assert(err, gc.IsNil)

	_, err = sendOperation.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	apiErr := &params.Error{Message: "already exists", Code: params.CodeAlreadyExists}
	select {
	case apiSender.errors <- apiErr:
	default:
		c.Fatalf("blocked error channel")
	}

	_, err = sendOperation.Execute(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(apiSender.batches, gc.HasLen, 1)

	reader, err := metrics.NewJSONMetricReader(s.spoolDir)
	c.Assert(err, gc.IsNil)
	batches, err := reader.Read()
	c.Assert(err, gc.IsNil)
	c.Assert(batches, gc.HasLen, 0)
}

func (s *MetricsOperationSuite) TestSendingFails(c *gc.C) {
	apiSender := newTestAPIMetricSender()

	factory := operation.NewFactory(operation.FactoryParams{
		MetricSender:   apiSender,
		MetricSpoolDir: s.spoolDir,
	})

	sendOperation, err := factory.NewSendMetrics()
	c.Assert(err, gc.IsNil)

	_, err = sendOperation.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case apiSender.sendError <- errors.New("something went wrong"):
	default:
		c.Fatalf("blocked error channel")
	}

	_, err = sendOperation.Execute(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(apiSender.batches, gc.HasLen, 1)

	reader, err := metrics.NewJSONMetricReader(s.spoolDir)
	c.Assert(err, gc.IsNil)
	batches, err := reader.Read()
	c.Assert(err, gc.IsNil)
	c.Assert(batches, gc.HasLen, 1)
}

func (s *MetricsOperationSuite) TestNoSpoolDirectory(c *gc.C) {
	apiSender := newTestAPIMetricSender()

	factory := operation.NewFactory(operation.FactoryParams{
		MetricSender:   apiSender,
		MetricSpoolDir: "/some/random/spool/dir",
	})

	sendOperation, err := factory.NewSendMetrics()
	c.Assert(err, gc.IsNil)

	_, err = sendOperation.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	_, err = sendOperation.Execute(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(apiSender.batches, gc.HasLen, 0)
}

func (s *MetricsOperationSuite) TestNoMetricsToSend(c *gc.C) {
	apiSender := newTestAPIMetricSender()

	newTmpSpoolDir := c.MkDir()

	factory := operation.NewFactory(operation.FactoryParams{
		MetricSender:   apiSender,
		MetricSpoolDir: newTmpSpoolDir,
	})

	sendOperation, err := factory.NewSendMetrics()
	c.Assert(err, gc.IsNil)

	_, err = sendOperation.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	_, err = sendOperation.Execute(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(apiSender.batches, gc.HasLen, 0)
}

func newTestAPIMetricSender() *testAPIMetricSender {
	return &testAPIMetricSender{errors: make(chan error, 1), sendError: make(chan error, 1)}
}

type testAPIMetricSender struct {
	batches   []params.MetricBatch
	errors    chan error
	sendError chan error
}

// AddMetricsBatches implements the operation.apiMetricsSender interface.
func (t *testAPIMetricSender) AddMetricBatches(batches []params.MetricBatch) (map[string]error, error) {
	t.batches = batches

	var err error
	select {
	case e := <-t.errors:
		err = e
	default:
		err = (*params.Error)(nil)
	}

	var sendErr error
	select {
	case e := <-t.sendError:
		sendErr = e
	default:
		sendErr = nil
	}

	errors := make(map[string]error)
	for _, b := range batches {
		errors[b.UUID] = err
	}
	return errors, sendErr
}
