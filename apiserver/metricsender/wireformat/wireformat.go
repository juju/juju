// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package wireformat defines the format that will be used to send metric
// batches to the collector and receive updates.
package wireformat

import (
	"time"

	"github.com/juju/juju/state"
)

// MetricBatch is a batch of metrics that will be sent to
// the metric collector
type MetricBatch struct {
	UUID     string    `json:"_id"`
	EnvUUID  string    `json:"env-uuid"`
	Unit     string    `json:"unit"`
	CharmUrl string    `json:"charm-url"`
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

// ToWire converts the state.MetricBatch into a type
// that can be sent over the wire to the collector.
func ToWire(mb *state.MetricBatch) *MetricBatch {
	metrics := make([]Metric, len(mb.Metrics()))
	for i, m := range mb.Metrics() {
		metrics[i] = Metric{
			Key:         m.Key,
			Value:       m.Value,
			Time:        m.Time.UTC(),
			Credentials: m.Credentials,
		}
	}
	return &MetricBatch{
		UUID:     mb.UUID(),
		EnvUUID:  mb.EnvUUID(),
		Unit:     mb.Unit(),
		CharmUrl: mb.CharmURL(),
		Created:  mb.Created().UTC(),
		Metrics:  metrics,
	}
}
