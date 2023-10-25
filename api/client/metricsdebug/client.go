// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides access to the metric debug api
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// MetricsDebugClient defines the methods on the metricsdebug API end point.
type MetricsDebugClient interface {
	// GetMetrics will receive metrics collected by the given entity tags
	// The tags act as a filter over what is to be returned. If no tags are
	// supplied GetMetrics will return all the metrics recorded in the
	// current model.
	GetMetrics(tags ...string) ([]params.MetricResult, error)
}

// MeterStatusClient defines methods on the metricsdebug API end point.
type MeterStatusClient interface {
	// SetMeterStatus will set the meter status on the given entity tag.
	SetMeterStatus(tag, code, info string) error
}

var _ MetricsDebugClient = (*Client)(nil)
var _ MeterStatusClient = (*Client)(nil)

// NewClient creates a new client for accessing the metricsdebug api
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, "MetricsDebug", options...)
	return &Client{ClientFacade: frontend, facade: backend}
}

// GetMetrics will receive metrics collected by the given entity
func (c *Client) GetMetrics(tags ...string) ([]params.MetricResult, error) {
	entities := make([]params.Entity, len(tags))
	for i, tag := range tags {
		entities[i] = params.Entity{Tag: tag}
	}
	p := params.Entities{Entities: entities}
	results := new(params.MetricResults)
	if err := c.facade.FacadeCall(context.TODO(), "GetMetrics", p, results); err != nil {
		return nil, errors.Trace(err)
	}
	if err := results.OneError(); err != nil {
		return nil, errors.Trace(err)
	}
	metrics := []params.MetricResult{}
	for _, r := range results.Results {
		metrics = append(metrics, r.Metrics...)
	}
	return metrics, nil
}

// SetMeterStatus will set the meter status on the given entity tag.
func (c *Client) SetMeterStatus(tag, code, info string) error {
	args := params.MeterStatusParams{
		Statuses: []params.MeterStatusParam{{
			Tag:  tag,
			Code: code,
			Info: info,
		},
		},
	}
	results := new(params.ErrorResults)
	if err := c.facade.FacadeCall(context.TODO(), "SetMeterStatus", args, results); err != nil {
		return errors.Trace(err)
	}
	if err := results.OneError(); err != nil {
		return errors.Trace(err)
	}
	return nil
}
