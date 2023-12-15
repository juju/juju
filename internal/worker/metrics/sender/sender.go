// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sender

import (
	"fmt"
	"net"
	"path"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/agent/metricsadder"
	csender "github.com/juju/juju/internal/sender"
	"github.com/juju/juju/internal/worker/metrics/spool"
	"github.com/juju/juju/rpc/params"
)

type stopper interface {
	Stop() error
}

type sender struct {
	client   metricsadder.MetricsAdderClient
	factory  spool.MetricFactory
	listener stopper
}

// Do sends metrics from the metric spool to the
// controller via an api call.
func (s *sender) Do(stop <-chan struct{}) (err error) {
	defer func() {
		// See bug https://pad/lv/1733469
		// If this function which is run by a PeriodicWorker
		// exits with an error, we need to call stop() to
		// ensure the sender socket is closed.
		if err != nil {
			s.stop()
		}
	}()

	reader, err := s.factory.Reader()
	if err != nil {
		return errors.Trace(err)
	}
	defer reader.Close()
	err = s.sendMetrics(reader)
	if spool.IsMetricsDataError(err) {
		logger.Debugf("cannot send metrics: %v", err)
		return nil
	}
	return err
}

func (s *sender) sendMetrics(reader spool.MetricReader) error {
	batches, err := reader.Read()
	if err != nil {
		return errors.Annotate(err, "failed to open the metric reader")
	}
	var sendBatches []params.MetricBatchParam
	for _, batch := range batches {
		sendBatches = append(sendBatches, spool.APIMetricBatch(batch))
	}
	results, err := s.client.AddMetricBatches(sendBatches)
	if err != nil {
		return errors.Annotate(err, "could not send metrics")
	}
	for batchUUID, resultErr := range results {
		// if we fail to send any metric batch we log a warning with the assumption that
		// the unsent metric batches remain in the spool directory and will be sent to the
		// controller when the network partition is restored.
		if _, ok := resultErr.(*params.Error); ok || params.IsCodeAlreadyExists(resultErr) {
			err := reader.Remove(batchUUID)
			if err != nil {
				logger.Errorf("could not remove batch %q from spool: %v", batchUUID, err)
			}
		} else {
			logger.Errorf("failed to send batch %q: %v", batchUUID, resultErr)
		}
	}
	return nil
}

// Handle sends metrics from the spool directory to the
// controller.
func (s *sender) Handle(c net.Conn, _ <-chan struct{}) (err error) {
	defer func() {
		if err != nil {
			fmt.Fprintf(c, "%v\n", err)
		} else {
			fmt.Fprintf(c, "ok\n")
		}
		c.Close()
	}()
	// TODO(fwereade): 2016-03-17 lp:1558657
	if err := c.SetDeadline(time.Now().Add(spool.DefaultTimeout)); err != nil {
		return errors.Annotate(err, "failed to set the deadline")
	}
	reader, err := s.factory.Reader()
	if err != nil {
		return errors.Trace(err)
	}
	defer reader.Close()
	return s.sendMetrics(reader)
}

func (s *sender) stop() {
	if s.listener != nil {
		_ = s.listener.Stop()
	}
}

var socketName = func(baseDir, unitTag string) string {
	return path.Join(baseDir, csender.DefaultMetricsSendSocketName)
}

func newSender(client metricsadder.MetricsAdderClient, factory spool.MetricFactory, baseDir, unitTag string) (*sender, error) {
	s := &sender{
		client:  client,
		factory: factory,
	}
	listener, err := newListener(s, baseDir, unitTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	s.listener = listener
	return s, nil
}

var newListener = func(s spool.ConnectionHandler, baseDir, unitTag string) (stopper, error) {
	listener, err := spool.NewSocketListener(socketName(baseDir, unitTag), s)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return listener, nil
}
