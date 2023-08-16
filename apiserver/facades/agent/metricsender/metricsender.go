// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	wireformat "github.com/juju/romulus/wireformat/metrics"

	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.metricsender")

// MetricSender defines the interface used to send metrics
// to a collection service.
type MetricSender interface {
	Send(context.Context, []*wireformat.MetricBatch) (*wireformat.Response, error)
}

type SenderFactory func(url string) MetricSender

var (
	defaultMaxBatchesPerSend               = 1000
	defaultSenderFactory     SenderFactory = func(url string) MetricSender {
		return &HTTPSender{url: url}
	}
)

func handleModelResponse(st ModelBackend, modelUUID string, modelResp wireformat.EnvResponse) int {
	err := st.SetMetricBatchesSent(modelResp.AcknowledgedBatches)
	if err != nil {
		logger.Errorf("failed to set sent on metrics %v", err)
	}
	for unitName, status := range modelResp.UnitStatuses {
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
	if modelResp.ModelStatus.Status != "" {
		err = st.SetModelMeterStatus(
			modelResp.ModelStatus.Status,
			modelResp.ModelStatus.Info,
		)
		if err != nil {
			logger.Errorf("failed to set the model meter status: %v", err)
		}
	}
	return len(modelResp.AcknowledgedBatches)
}

func handleResponse(mm *state.MetricsManager, st ModelBackend, response wireformat.Response) int {
	var acknowledgedBatches int
	for modelUUID, modelResp := range response.EnvResponses {
		acks := handleModelResponse(st, modelUUID, modelResp)
		acknowledgedBatches += acks
	}
	if response.NewGracePeriod > 0 && response.NewGracePeriod != mm.GracePeriod() {
		err := mm.SetGracePeriod(response.NewGracePeriod)
		if err != nil {
			logger.Errorf("failed to set new grace period %v", err)
		}
	}
	return acknowledgedBatches
}

// SendMetrics will send any unsent metrics
// over the MetricSender interface in batches
// no larger than batchSize.
func SendMetrics(st ModelBackend, sender MetricSender, clock clock.Clock, batchSize int, transmitVendorMetrics bool) error {
	metricsManager, err := st.MetricsManager()
	if err != nil {
		return errors.Trace(err)
	}
	sent := 0
	held := 0
	for {
		metrics, err := st.MetricsToSend(batchSize)
		if err != nil {
			return errors.Trace(err)
		}
		modelName := st.Name()
		lenM := len(metrics)
		if lenM == 0 {
			if sent == 0 {
				logger.Debugf("nothing to send")
			} else {
				logger.Debugf("done sending")
			}
			break
		}

		var wireData []*wireformat.MetricBatch
		var heldBatches []string
		heldBatchUnits := map[string]bool{}
		for _, m := range metrics {
			if !transmitVendorMetrics && len(m.Credentials()) == 0 {
				heldBatches = append(heldBatches, m.UUID())
				heldBatchUnits[m.Unit()] = true
			} else {
				wireData = append(wireData, ToWire(m, modelName))
			}
		}
		response, err := sender.Send(context.Background(), wireData)
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
			acknowledged := handleResponse(metricsManager, st, *response)
			// Stop sending if there are no acknowledged batches.
			if acknowledged == 0 {
				logger.Debugf("got 0 acks, ending send loop")
				break
			}
			if err := metricsManager.SetLastSuccessfulSend(clock.Now()); err != nil {
				err = errors.Annotate(err, "failed to set successful send time")
				logger.Warningf("%v", err)
				return errors.Trace(err)
			}
		}
		// Mark held metric batches as sent so that they can be cleaned up later.
		if len(heldBatches) > 0 {
			err := st.SetMetricBatchesSent(heldBatches)
			if err != nil {
				return errors.Annotatef(err, "failed to mark metric batches as sent for %s", st.ModelTag())
			}
		}

		setHeldBatchUnitMeterStatus(st, heldBatchUnits)

		sent += len(wireData)
		held += len(heldBatches)
	}

	unsent, err := st.CountOfUnsentMetrics()
	if err != nil {
		return errors.Trace(err)
	}
	sentStored, err := st.CountOfSentMetrics()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("metrics collection summary for %s: sent:%d unsent:%d held:%d (%d sent metrics stored)", st.ModelTag(), sent, unsent, held, sentStored)

	return nil
}

func setHeldBatchUnitMeterStatus(st ModelBackend, units map[string]bool) {
	for unitID := range units {
		unit, err := st.Unit(unitID)
		if err != nil {
			logger.Warningf("failed to get unit for setting held batch meter status: %v", err)
		}
		if err = unit.SetMeterStatus("RED", "transmit-vendor-metrics turned off"); err != nil {
			logger.Warningf("failed to set held batch meter status: %v", err)
		}
	}
}

// DefaultMaxBatchesPerSend returns the default number of batches per send.
func DefaultMaxBatchesPerSend() int {
	return defaultMaxBatchesPerSend
}

// DefaultSenderFactory returns the default sender factory.
func DefaultSenderFactory() SenderFactory {
	return defaultSenderFactory
}

// ToWire converts the state.MetricBatch into a type
// that can be sent over the wire to the collector.
func ToWire(mb *state.MetricBatch, modelName string) *wireformat.MetricBatch {
	metrics := make([]wireformat.Metric, len(mb.Metrics()))
	for i, m := range mb.Metrics() {
		metrics[i] = wireformat.Metric{
			Key:    m.Key,
			Value:  m.Value,
			Time:   m.Time.UTC(),
			Labels: m.Labels,
		}
	}
	return &wireformat.MetricBatch{
		UUID:           mb.UUID(),
		ModelUUID:      mb.ModelUUID(),
		ModelName:      modelName,
		UnitName:       mb.Unit(),
		CharmUrl:       mb.CharmURL(),
		Created:        mb.Created().UTC(),
		Metrics:        metrics,
		Credentials:    mb.Credentials(),
		SLACredentials: mb.SLACredentials(),
	}
}
