// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"
)

// MetricResults contains results from a GetMetrics call, with
// one item per Entity given as an argument to the command.
type MetricResults struct {
	Results []EntityMetrics `json:"results"`
}

// OneError returns the first error
func (m *MetricResults) OneError() error {
	for _, r := range m.Results {
		if err := r.Error; err != nil {
			return err
		}
	}
	return nil
}

// EntityMetrics contains the results of a GetMetrics call for a single
// entity.
type EntityMetrics struct {
	Metrics []MetricResult `json:"metrics,omitempty"`
	Error   *Error         `json:"error,omitempty"`
}

// MetricResults contains a single metric.
type MetricResult struct {
	Time  time.Time `json:"time"`
	Key   string    `json:"key"`
	Value string    `json:"value"`
}
