// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsadder contains an implementation of the api facade to
// add metrics to the state.
package metricsadder

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// MetricsAdderClient defines the methods on the metricadder API end point.
type MetricsAdderClient interface {
	// AddMetricBatches stores specified metric batches in the state.
	AddMetricBatches(batches []params.MetricBatchParam) (map[string]error, error)
}

// NewClient creates a new client for accessing the metricsadder API.
func NewClient(caller base.APICaller) *Client {
	return &Client{facade: base.NewFacadeCaller(caller, "MetricsAdder")}
}

var _ MetricsAdderClient = (*Client)(nil)

// Client provides access to the metrics adder API.
type Client struct {
	facade base.FacadeCaller
}

// AddMetricBatches implements the MetricsAdderClient interface.
func (c *Client) AddMetricBatches(batches []params.MetricBatchParam) (map[string]error, error) {
	parameters := params.MetricBatchParams{
		Batches: batches,
	}
	results := new(params.ErrorResults)
	err := c.facade.FacadeCall("AddMetricBatches", parameters, results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	resultMap := make(map[string]error)
	for i, result := range results.Results {
		resultMap[batches[i].Batch.UUID] = result.Error
	}
	return resultMap, nil
}
