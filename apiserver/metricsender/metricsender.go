// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsender contains types and functions for sending
// metrics from a state server to a remote metric collector.
package metricsender

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/state"
)

var sendLogger = loggo.GetLogger("juju.apiserver.metricsender")

type MetricBatch struct {
	UUID     string    `json:"_id"`
	EnvUUID  string    `json:"envuuid"`
	Unit     string    `json:"unit"`
	CharmUrl string    `json:"charmurl"`
	Created  time.Time `json:"created"`
	Metrics  []Metric  `json:"metrics"`
}

// Metric represents a single Metric.
type Metric struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Time        time.Time `json:"time"`
	Credentials []byte    `json:"credentials"`
}

// MetricSender defines the interface used to send metrics
// to a collection service.
type MetricSender interface {
	Send([]*MetricBatch) error
}

// ToWire converts the state.MetricBatch into a type
// that can be sent over the wire to the collector.
func ToWire(mb *state.MetricBatch) *MetricBatch {
	metrics := make([]Metric, len(mb.Metrics()))
	for i, m := range mb.Metrics() {
		metrics[i] = Metric{
			Key:         m.Key,
			Value:       m.Value,
			Time:        m.Time,
			Credentials: m.Credentials,
		}
	}
	return &MetricBatch{
		UUID:     mb.UUID(),
		EnvUUID:  mb.EnvUUID(),
		Unit:     mb.Unit(),
		CharmUrl: mb.CharmURL(),
		Created:  mb.Created(),
		Metrics:  metrics,
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
			sendLogger.Infof("nothing to send")
			break
		}
		wireData := make([]*MetricBatch, len(metrics))
		for i, m := range metrics {
			wireData[i] = ToWire(m)
		}
		err = sender.Send(wireData)
		if err != nil {
			return errors.Trace(err)
		}
		err = st.SetMetricBatchesSent(metrics)
		if err != nil {
			sendLogger.Warningf("failed to set sent on metrics %v", err)
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
	sendLogger.Infof("metrics collection summary: sent:%d unsent:%d", sent, unsent)

	return nil
}
