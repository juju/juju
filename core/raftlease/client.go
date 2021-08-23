// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/core/lease"
	"github.com/juju/pubsub/v2"
	"github.com/juju/utils/v2"
	"github.com/prometheus/client_golang/prometheus"
)

// Client defines the methods for broadcasting a command.
type Client interface {
	Request(context.Context, *Command) error
}

// ClientMetrics represents the metrics during a client request.
type ClientMetrics interface {
	RecordOperation(string, string, time.Time)
}

type PubsubClient struct {
	hub            *pubsub.StructuredHub
	requestID      uint64
	requestTopic   string
	metrics        ClientMetrics
	forwardTimeout time.Duration
	clock          clock.Clock
}

// PubsubClientConfig holds resources and settings needed to run the
// PubsubClient.
type PubsubClientConfig struct {
	Hub            *pubsub.StructuredHub
	RequestTopic   string
	ClientMetrics  ClientMetrics
	Clock          clock.Clock
	ForwardTimeout time.Duration
}

// NewPubsubClient creates a PubSub raftlease client.
func NewPubsubClient(config PubsubClientConfig) *PubsubClient {
	return &PubsubClient{
		hub:            config.Hub,
		requestTopic:   config.RequestTopic,
		metrics:        config.ClientMetrics,
		forwardTimeout: config.ForwardTimeout,
		clock:          config.Clock,
	}
}

func (c *PubsubClient) Request(ctx context.Context, command *Command) error {
	bytes, err := command.Marshal()
	if err != nil {
		return errors.Trace(err)
	}

	start := time.Now()

	responseTopic := utils.MustNewUUID().String()

	responseChan := make(chan ForwardResponse)
	errChan := make(chan error)
	unsubscribe, err := c.hub.Subscribe(
		responseTopic,
		func(_ string, resp ForwardResponse, err error) {
			if err != nil {
				errChan <- err
			} else {
				responseChan <- resp
			}
		},
	)
	if err != nil {
		return errors.Annotatef(err, "running %s", command)
	}
	defer unsubscribe()

	delivered, err := c.hub.Publish(c.requestTopic, ForwardRequest{
		Command:       string(bytes),
		ResponseTopic: responseTopic,
	})
	if err != nil {
		c.record(command.Operation, "error", start)
		return errors.Annotatef(err, "publishing %s", command)
	}

	// First block until subscribers are notified.
	// In practice, this will be the Raft forwarder running on the leader node.
	// This is an explicit step so that we can more accurately diagnose issues
	// in-theatre.
	select {
	case <-pubsub.Wait(delivered):
	case <-c.clock.After(c.forwardTimeout):
		logger.Warningf("delivery timeout waiting for %s to be processed", command)
		c.record(command.Operation, "delivery timeout", start)
		return lease.ErrTimeout
	}

	// Now wait for the response.
	// The timeout starts again here, which is deliberate.
	// It is the same timeout that is used by the Raft forwarder
	// when `Apply` is called on the FSM.
	select {
	case response := <-responseChan:
		err := RecoverError(response.Error)
		logger.Tracef("got response, err %v", err)
		result := "success"
		if err != nil {
			logger.Warningf("command %s: %v", command, err)
			result = "failure"
		}
		c.record(command.Operation, result, start)
		return err
	case err := <-errChan:
		logger.Warningf("processing %s: %v", command, err)
		c.record(command.Operation, "error", start)
		return errors.Trace(err)
	case <-ctx.Done():
		return aborted(command)
	case <-c.clock.After(c.forwardTimeout):
		// TODO (thumper) 2019-12-20, bug 1857072
		// Scale testing hit this a *lot*,
		// perhaps we need to consider batching messages to run on the leader?
		logger.Warningf("response timeout waiting for %s to be processed", command)
		c.record(command.Operation, "response timeout", start)
		return lease.ErrTimeout
	}
}

func (c PubsubClient) record(operation, result string, start time.Time) {
	c.metrics.RecordOperation(operation, result, start)
}

type OperationClientMetrics struct {
	metrics *metricsCollector
	clock   clock.Clock
}

func NewOperationClientMetrics(clock clock.Clock) *OperationClientMetrics {
	return &OperationClientMetrics{
		metrics: newMetricsCollector(),
		clock:   clock,
	}
}

func (m OperationClientMetrics) RecordOperation(operation, result string, start time.Time) {
	elapsedMS := float64(m.clock.Now().Sub(start)) / float64(time.Millisecond)
	m.metrics.requests.With(prometheus.Labels{
		"operation": operation,
		"result":    result,
	}).Observe(elapsedMS)
}

// Describe is part of prometheus.Collector.
func (c *OperationClientMetrics) Describe(ch chan<- *prometheus.Desc) {
	c.metrics.Describe(ch)
}

// Collect is part of prometheus.Collector.
func (c *OperationClientMetrics) Collect(ch chan<- prometheus.Metric) {
	c.metrics.Collect(ch)
}
