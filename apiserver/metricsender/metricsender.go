// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsender contains functions for sending
// metrics from a controller to a remote metric collector.
package metricsender

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	wireformat "github.com/juju/romulus/wireformat/metrics"

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
	defaultSender            MetricSender = &HttpSender{}
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
	sent := 0
	for {
		metrics, err := st.MetricsToSend(batchSize)
		if err != nil {
			return errors.Trace(err)
		}
		lenM := len(metrics)
		if lenM == 0 {
			if sent == 0 {
				logger.Infof("nothing to send")
			} else {
				logger.Infof("done sending")
			}
			break
		}
		wireData := make([]*wireformat.MetricBatch, lenM)
		for i, m := range metrics {
			wireData[i] = ToWire(m)
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
		sent += lenM
	}

	unsent, err := st.CountOfUnsentMetrics()
	if err != nil {
		return errors.Trace(err)
	}
	sentStored, err := st.CountOfSentMetrics()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Infof("metrics collection summary: sent:%d unsent:%d (%d sent metrics stored)", sent, unsent, sentStored)

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

// ToWire converts the state.MetricBatch into a type
// that can be sent over the wire to the collector.
func ToWire(mb *state.MetricBatch) *wireformat.MetricBatch {
	metrics := make([]wireformat.Metric, len(mb.Metrics()))
	for i, m := range mb.Metrics() {
		metrics[i] = wireformat.Metric{
			Key:   m.Key,
			Value: m.Value,
			Time:  m.Time.UTC(),
		}
	}
	return &wireformat.MetricBatch{
		UUID:        mb.UUID(),
		ModelUUID:   mb.ModelUUID(),
		UnitName:    mb.Unit(),
		CharmUrl:    mb.CharmURL(),
		Created:     mb.Created().UTC(),
		Metrics:     metrics,
		Credentials: mb.Credentials(),
	}
}
