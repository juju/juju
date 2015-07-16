// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"
	"time"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/metrics"
)

// apiMetricSender is used to send metrics to the state server. Its default implementation is
// *uniter.Unit.
type apiMetricSender interface {
	AddMetricBatches(batches []params.MetricBatch) (map[string]error, error)
}

// metricsReader is used to read metrics batches stored by the metrics recorder
// and remove metrics batches that have been marked as succesfully sent.
type metricsReader interface {
	Read() ([]metrics.MetricBatch, error)
	Remove(uuid string) error
	Close() error
}

type sendMetrics struct {
	DoesNotRequireMachineLock
	spoolDir string
	sender   apiMetricSender
}

// String implements the Operation interface.
func (op *sendMetrics) String() string {
	return fmt.Sprintf("sending metrics")
}

// Prepare implements the Operation interface.
func (op *sendMetrics) Prepare(state State) (*State, error) {
	return &state, nil
}

// Execute implements the Operation interface.
// Execute will try to read any metric batches stored in the spool directory
// and send them to the state server.
func (op *sendMetrics) Execute(state State) (*State, error) {
	reader, err := metrics.NewJSONMetricReader(op.spoolDir)
	if err != nil {
		logger.Warningf("failed to create a metric reader: %v", err)
		return &state, nil
	}

	batches, err := reader.Read()
	if err != nil {
		logger.Warningf("failed to open the metric reader: %v", err)
		return &state, nil
	}
	defer reader.Close()
	var sendBatches []params.MetricBatch
	for _, batch := range batches {
		sendBatches = append(sendBatches, metrics.APIMetricBatch(batch))
	}
	results, err := op.sender.AddMetricBatches(sendBatches)
	if err != nil {
		logger.Warningf("could not send metrics: %v", err)
		return &state, nil
	}
	for batchUUID, resultErr := range results {
		// if we fail to send any metric batch we log a warning with the assumption that
		// the unsent metric batches remain in the spool directory and will be sent to the
		// state server when the network partition is restored.
		if _, ok := resultErr.(*params.Error); ok || params.IsCodeAlreadyExists(resultErr) {
			err = reader.Remove(batchUUID)
			if err != nil {
				logger.Warningf("could not remove batch %q from spool: %v", batchUUID, err)
			}
		} else {
			logger.Warningf("failed to send batch %q: %v", batchUUID, resultErr)
		}
	}
	return &state, nil
}

// Commit implements the Operation interface.
func (op *sendMetrics) Commit(state State) (*State, error) {
	state.SendMetricsTime = time.Now().Unix()
	return &state, nil
}
