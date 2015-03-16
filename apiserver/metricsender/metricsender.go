// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsender contains functions for sending
// metrics from a state server to a remote metric collector.
package metricsender

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/metricsender/wireformat"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.metricsender")

// MetricSender defines the interface used to send metrics
// to a collection service.
type MetricSender interface {
	Send([]*wireformat.MetricBatch) (*wireformat.Response, error)
}

func handleResponse(st *state.State, response wireformat.Response) {
	for _, envResp := range response.EnvResponses {
		err := st.SetMetricBatchesSent(envResp.AcknowledgedBatches)
		if err != nil {
			logger.Errorf("failed to set sent on metrics %v", err)
		}
		for unitName, status := range envResp.UnitStatuses {
			unit, err := st.Unit(unitName)
			if err != nil {
				logger.Errorf("failed to retrieve unit %q: %v", unitName, err)
				continue
			}
			err = unit.SetMeterStatus(status.Status, status.Info)
			if err != nil {
				logger.Errorf("failed to set unit %q meter status to %v: %v", unitName, status, err)
			}
		}
	}
	if response.NewGracePeriod > 0 {
		mm, err := st.MetricsManager()
		if err != nil {
			logger.Errorf("failed to set new grace period %v", err)
		}
		err = mm.SetGracePeriod(response.NewGracePeriod)
		if err != nil {
			logger.Errorf("failed to set new grace period %v", err)
		}
	}
}

// SendMetrics will send any unsent metrics
// over the MetricSender interface in batches
// no larger than batchSize.
func SendMetrics(st *state.State, sender MetricSender, batchSize int) error {
	for {
		metrics, err := st.MetricsToSend(batchSize)
		if err != nil {
			return errors.Trace(err)
		}
		if len(metrics) == 0 {
			logger.Infof("nothing to send")
			break
		}
		wireData := make([]*wireformat.MetricBatch, len(metrics))
		for i, m := range metrics {
			wireData[i] = wireformat.ToWire(m)
		}
		response, err := sender.Send(wireData)
		if err != nil {
			logger.Errorf("%+v", err)
			return errors.Trace(err)
		}
		if response != nil {
			// TODO (mattyw) We are currently ignoring errors during response handling.
			handleResponse(st, *response)
		}
	}

	unsent, err := st.CountofUnsentMetrics()
	if err != nil {
		return errors.Trace(err)
	}
	sent, err := st.CountofSentMetrics()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Infof("metrics collection summary: sent:%d unsent:%d", sent, unsent)

	return nil
}
