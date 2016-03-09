// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sender contains the implementation of the metric
// sender manifold.
package sender

import (
	"fmt"
	"net"
	"path"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/os"

	"github.com/juju/juju/api/metricsadder"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/metrics/spool"
)

const (
	defaultSocketName = "metrics-send.socket"
)

type stopper interface {
	Stop()
}

type sender struct {
	client   metricsadder.MetricsAdderClient
	factory  spool.MetricFactory
	listener stopper
}

// Do sends metrics from the metric spool to the
// controller via an api call.
func (s *sender) Do(stop <-chan struct{}) error {
	reader, err := s.factory.Reader()
	if err != nil {
		return errors.Trace(err)
	}
	defer reader.Close()
	return s.sendMetrics(reader)
}

func (s *sender) sendMetrics(reader spool.MetricReader) error {
	batches, err := reader.Read()
	if err != nil {
		logger.Warningf("failed to open the metric reader: %v", err)
		return errors.Trace(err)
	}
	var sendBatches []params.MetricBatchParam
	for _, batch := range batches {
		sendBatches = append(sendBatches, spool.APIMetricBatch(batch))
	}
	results, err := s.client.AddMetricBatches(sendBatches)
	if err != nil {
		logger.Warningf("could not send metrics: %v", err)
		return errors.Trace(err)
	}
	for batchUUID, resultErr := range results {
		// if we fail to send any metric batch we log a warning with the assumption that
		// the unsent metric batches remain in the spool directory and will be sent to the
		// controller when the network partition is restored.
		if _, ok := resultErr.(*params.Error); ok || params.IsCodeAlreadyExists(resultErr) {
			err = reader.Remove(batchUUID)
			if err != nil {
				logger.Warningf("could not remove batch %q from spool: %v", batchUUID, err)
			}
		} else {
			logger.Warningf("failed to send batch %q: %v", batchUUID, resultErr)
		}
	}
	return nil
}

// Handle sends metrics from the spool directory to the
// controller.
func (s *sender) Handle(c net.Conn) (err error) {
	defer func() {
		if err != nil {
			fmt.Fprintf(c, "%v\n", err)
		} else {
			fmt.Fprintf(c, "ok\n")
		}
		c.Close()
	}()
	err = c.SetDeadline(time.Now().Add(spool.DefaultTimeout))
	if err != nil {
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
		s.listener.Stop()
	}
}

var socketName = func(baseDir, unitTag string) string {
	if os.HostOS() == os.Windows {
		return fmt.Sprintf(`\\.\pipe\send-metrics-%s`, unitTag)
	}
	return path.Join(baseDir, defaultSocketName)
}

func newSender(client metricsadder.MetricsAdderClient, factory spool.MetricFactory, baseDir, unitTag string) (*sender, error) {
	s := &sender{
		client:  client,
		factory: factory,
	}
	listener, err := spool.NewSocketListener(socketName(baseDir, unitTag), s)
	if err != nil {
		return nil, errors.Trace(err)
	}
	s.listener = listener
	return s, nil
}
