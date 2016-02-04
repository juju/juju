// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sender contains the implementation of the metric
// sender manifold.
package sender

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/metricsadder"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/metrics/spool"
)

type sender struct {
	client  metricsadder.MetricsAdderClient
	factory spool.MetricFactory
}

// Do sends metrics from the metric spool to the
// controller via an api call.
func (s *sender) Do(stop <-chan struct{}) error {
	reader, err := s.factory.Reader()
	if err != nil {
		return errors.Trace(err)
	}
	batches, err := reader.Read()
	if err != nil {
		logger.Warningf("failed to open the metric reader: %v", err)
		return errors.Trace(err)
	}
	defer reader.Close()
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

func newSender(client metricsadder.MetricsAdderClient, factory spool.MetricFactory) sender {
	return sender{
		client:  client,
		factory: factory,
	}
}
