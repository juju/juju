// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// metricstorage contains implementation for the API client accessing
// the metricstorage API facade.
package metricstorage

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client provides access to the metric storage api.
type Client struct {
	base.ClientFacade
	st     *api.State
	facade base.FacadeCaller
	tag    names.UnitTag
}

// NewClient creates a new client for accessing the metricstorage api
func NewClient(st *api.State, tag names.UnitTag) *Client {
	frontend, backend := base.NewClientFacade(st, "MetricStorage")
	return &Client{ClientFacade: frontend, st: st, facade: backend, tag: tag}
}

// AddMetricBatches stores the provided metric batches in state.
func (c *Client) AddMetricBatches(batches []params.MetricBatch) error {
	p := params.MetricBatchParams{
		Batches: make([]params.MetricBatchParam, len(batches)),
	}

	for i, batch := range batches {
		p.Batches[i].Tag = c.tag.String()
		p.Batches[i].Batch = batch
	}
	results := new(params.ErrorResults)
	err := c.facade.FacadeCall("AddMetricBatches", p, results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.Combine()
}
