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
	UUID        string    `json:"uuid"`
	EnvUUID     string    `json:"env-uuid"`
	UnitName    string    `json:"unit-name"`
	CharmUrl    string    `json:"charm-url"`
	Created     time.Time `json:"created"`
	Metrics     []Metric  `json:"metrics"`
	Credentials []byte    `json:"credentials"`
}

// Metric represents a single Metric.
type Metric struct {
	Key   string    `json:"key"`
	Value string    `json:"value"`
	Time  time.Time `json:"time"`
}

// ToWire converts the state.MetricBatch into a type
// that can be sent over the wire to the collector.
func ToWire(mb *state.MetricBatch) *MetricBatch {
	metrics := make([]Metric, len(mb.Metrics()))
	for i, m := range mb.Metrics() {
		metrics[i] = Metric{
			Key:   m.Key,
			Value: m.Value,
			Time:  m.Time.UTC(),
		}
	}
	return &MetricBatch{
		UUID:        mb.UUID(),
		EnvUUID:     mb.EnvUUID(),
		UnitName:    mb.Unit(),
		CharmUrl:    mb.CharmURL(),
		Created:     mb.Created().UTC(),
		Metrics:     metrics,
		Credentials: mb.Credentials(),
	}
}

// Response represents the response from the metrics collector.
type Response struct {
	UUID           string               `json:"uuid"`
	EnvResponses   EnvironmentResponses `json:"env-responses"`
	NewGracePeriod time.Duration        `json:"new-grace-period"`
}

type EnvironmentResponses map[string]EnvResponse

// Ack adds the specified the batch UUID to the list of acknowledged batches
// for the specified environment.
func (e EnvironmentResponses) Ack(envUUID, batchUUID string) {
	env := e[envUUID]

	env.AcknowledgedBatches = append(env.AcknowledgedBatches, batchUUID)
	e[envUUID] = env
}

func (e EnvironmentResponses) SetStatus(envUUID, unitName, status, info string) {
	s := UnitStatus{
		Status: status,
		Info:   info,
	}

	env := e[envUUID]

	if env.UnitStatuses == nil {
		env.UnitStatuses = map[string]UnitStatus{
			unitName: s,
		}
	} else {
		env.UnitStatuses[unitName] = s
	}
	e[envUUID] = env

}

// EnvResponse contains the response data relevant to a concrete environment.
type EnvResponse struct {
	AcknowledgedBatches []string              `json:"acks,omitempty"`
	UnitStatuses        map[string]UnitStatus `json:"unit-statuses,omitempty"`
}

type UnitStatus struct {
	Status string `json:"status"`
	Info   string `json:"info"`
}
