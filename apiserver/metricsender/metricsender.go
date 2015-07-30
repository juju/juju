// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsender contains functions for sending
// metrics from a state server to a remote metric collector.
package metricsender

import (
	"time"

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

var (
	defaultMaxBatchesPerSend              = 10
	defaultSender            MetricSender = &NopSender{}
)

func handleResponse(mm *state.MetricsManager, st *state.State, response wireformat.Response) {
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
		err := mm.SetGracePeriod(response.NewGracePeriod)
		if err != nil {
			logger.Errorf("failed to set new grace period %v", err)
		}
	}
}

// SendMetrics will send any unsent metrics
// over the MetricSender interface in batches
// no larger than batchSize.
func SendMetrics(st *state.State, sender MetricSender, batchSize int) error {
	metricsManager, err := st.MetricsManager()
	if err != nil {
		return errors.Trace(err)
	}
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
			if incErr := metricsManager.IncrementConsecutiveErrors(); incErr != nil {
				logger.Errorf("failed to increment error count %v", incErr)
				return errors.Trace(errors.Wrap(err, incErr))
			}
			return errors.Trace(err)
		}
		if response != nil {
			// TODO (mattyw) We are currently ignoring errors during response handling.
			handleResponse(metricsManager, st, *response)
			if err := metricsManager.SetLastSuccessfulSend(time.Now()); err != nil {
				err = errors.Annotate(err, "failed to set successful send time")
				logger.Warningf("%v", err)
				return errors.Trace(err)
			}
		}
	}

	unsent, err := st.CountOfUnsentMetrics()
	if err != nil {
		return errors.Trace(err)
	}
	sent, err := st.CountOfSentMetrics()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Infof("metrics collection summary: sent:%d unsent:%d", sent, unsent)

	return nil
}

// DefaultMaxBatchesPerSend returns the default number of batches per send.
func DefaultMaxBatchesPerSend() int {
	return defaultMaxBatchesPerSend
}

// DefaultMetricSender returns the default metric sender.
func DefaultMetricSender() MetricSender {
	return defaultSender
}
