// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metrics defines the format that will be used to send metric
// batches to the collector and receive updates.
package metrics

import (
	"time"
)

// MetricBatch is a batch of metrics that will be sent to
// the metric collector
type MetricBatch struct {
	UUID                string    `json:"uuid"`
	ModelUUID           string    `json:"env-uuid"`
	ModelName           string    `json:"model-name"`
	UnitName            string    `json:"unit-name"`
	CharmUrl            string    `json:"charm-url"`
	Created             time.Time `json:"created"`
	Metrics             []Metric  `json:"metrics"`
	Credentials         []byte    `json:"credentials"`
	ResellerCredentials []byte    `json:"reseller-credentials"`
	SLACredentials      []byte    `json:"sla-credentials"`
}

// Metric represents a single Metric.
type Metric struct {
	Key    string            `json:"key"`
	Value  string            `json:"value"`
	Time   time.Time         `json:"time"`
	Labels map[string]string `json:"labels,omitempty"`
}

// Response represents the response from the metrics collector.
type Response struct {
	UUID           string               `json:"uuid"`
	EnvResponses   EnvironmentResponses `json:"env-responses"`
	NewGracePeriod time.Duration        `json:"new-grace-period"`
}

// EnvironmentResponses is a map of model UUID to wireformat responses.
type EnvironmentResponses map[string]EnvResponse

// Ack adds the specified the batch UUID to the list of acknowledged batches
// for the specified environment.
// If the model UUID or batch UUID is nil, the batch will not be acknowledged.
func (e EnvironmentResponses) Ack(modelUUID, batchUUID string) {
	if modelUUID == "" || batchUUID == "" {
		return // Inability to ack is a response too.
	}
	env := e[modelUUID]
	env.AcknowledgedBatches = append(env.AcknowledgedBatches, batchUUID)
	e[modelUUID] = env
}

// SetUnitStatus sets the unit meter status.
func (e EnvironmentResponses) SetUnitStatus(modelUUID, unitName, status, info string) {
	s := UnitStatus{
		Status: status,
		Info:   info,
	}

	env := e[modelUUID]

	if env.UnitStatuses == nil {
		env.UnitStatuses = map[string]UnitStatus{
			unitName: s,
		}
	} else {
		env.UnitStatuses[unitName] = s
	}
	e[modelUUID] = env
}

// SetModelStatus sets the model meter status.
func (e EnvironmentResponses) SetModelStatus(modelUUID, status, info string) {
	s := ModelStatus{
		Status: status,
		Info:   info,
	}
	env := e[modelUUID]
	env.ModelStatus = s
	e[modelUUID] = env
}

// EnvResponse contains the response data relevant to a concrete environment.
type EnvResponse struct {
	AcknowledgedBatches []string              `json:"acks,omitempty"`
	ModelStatus         ModelStatus           `json:"model-status,omitempty"`
	UnitStatuses        map[string]UnitStatus `json:"unit-statuses,omitempty"`
}

// ModelStatus represents the status of a model.
type ModelStatus struct {
	Status string `json:"status"`
	Info   string `json:"info"`
}

// UnitStatus represents the status of a unit.
type UnitStatus struct {
	Status string `json:"status"`
	Info   string `json:"info"`
}
