// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/rpc"
)

// MetricLabels used for setting labels for the Counter and Summary vectors.
const (
	MetricLabelFacade    = "facade"
	MetricLabelVersion   = "version"
	MetricLabelMethod    = "method"
	MetricLabelErrorCode = "error_code"
)

// MetricLabelNames holds the names for reporting the names of the metric
// types when calling the observers.
var MetricLabelNames = []string{
	MetricLabelFacade,
	MetricLabelVersion,
	MetricLabelMethod,
	MetricLabelErrorCode,
}

// CounterVec is a Collector that bundles a set of Counters that all share the
// same description.
type CounterVec interface {
	// With returns a Counter for a given labels slice
	With(prometheus.Labels) prometheus.Counter
}

// SummaryVec is a Collector that bundles a set of Summaries that all share the
// same description.
type SummaryVec interface {
	// With returns a Summary for a given labels slice
	With(prometheus.Labels) prometheus.Observer
}

// MetricsCollector represents a bundle of metrics that is used by the observer
// factory.
//go:generate mockgen -package mocks -destination mocks/metrics_collector_mock.go github.com/juju/juju/apiserver/observer/metricobserver MetricsCollector,CounterVec,SummaryVec
//go:generate mockgen -package mocks -destination mocks/metrics_mock.go github.com/prometheus/client_golang/prometheus Counter,Summary
type MetricsCollector interface {
	// APIRequestDuration returns a SummaryVec for updating the duration of
	// api request duration.
	APIRequestDuration() SummaryVec

	// DeprecatedAPIRequestsTotal returns a CounterVec for updating the number of
	// api requests total.
	// The following is obsolete and should be removed for 2.6 release
	DeprecatedAPIRequestsTotal() CounterVec

	// DeprecatedAPIRequestDuration returns a SummaryVec for updating the duration of
	// api request duration.
	// The following is obsolete and should be removed for 2.6 release
	DeprecatedAPIRequestDuration() SummaryVec
}

// Config contains the configuration for an Observer.
type Config struct {
	// Clock is the clock to use for all time-related operations.
	Clock clock.Clock

	// MetricsCollector defines .
	MetricsCollector MetricsCollector
}

// Validate validates the observer factory configuration.
func (cfg Config) Validate() error {
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.MetricsCollector == nil {
		return errors.NotValidf("nil MetricsCollector")
	}
	return nil
}

// NewObserverFactory returns a function that, when called, returns a new
// Observer. NewObserverFactory registers the API request metrics, and
// each Observer updates those metrics.
func NewObserverFactory(config Config) (observer.ObserverFactory, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating config")
	}

	// Observer is currently stateless, so we return the same one for each
	// API connection. Individual RPC requests still get their own RPC
	// observers.
	o := &Observer{
		clock: config.Clock,
		metrics: metrics{
			apiRequestDuration:           config.MetricsCollector.APIRequestDuration(),
			deprecatedAPIRequestsTotal:   config.MetricsCollector.DeprecatedAPIRequestsTotal(),
			deprecatedAPIRequestDuration: config.MetricsCollector.DeprecatedAPIRequestDuration(),
		},
	}
	return func() observer.Observer {
		return o
	}, nil
}

// Observer is an API server request observer that collects Prometheus metrics.
type Observer struct {
	clock   clock.Clock
	metrics metrics
}

type metrics struct {
	apiRequestDuration           SummaryVec
	deprecatedAPIRequestDuration SummaryVec
	deprecatedAPIRequestsTotal   CounterVec
}

// Login is part of the observer.Observer interface.
func (*Observer) Login(entity names.Tag, _ names.ModelTag, _ bool, _ string) {}

// Join is part of the observer.Observer interface.
func (*Observer) Join(req *http.Request, connectionID uint64) {}

// Leave is part of the observer.Observer interface.
func (*Observer) Leave() {}

// RPCObserver is part of the observer.Observer interface.
func (o *Observer) RPCObserver() rpc.Observer {
	return &rpcObserver{
		clock:   o.clock,
		metrics: o.metrics,
	}
}

type rpcObserver struct {
	clock        clock.Clock
	metrics      metrics
	requestStart time.Time
}

// ServerRequest is part of the rpc.Observer interface.
func (o *rpcObserver) ServerRequest(hdr *rpc.Header, body interface{}) {
	o.requestStart = o.clock.Now()
}

// ServerReply is part of the rpc.Observer interface.
func (o *rpcObserver) ServerReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
	labels := prometheus.Labels{
		MetricLabelFacade:    req.Type,
		MetricLabelVersion:   strconv.Itoa(req.Version),
		MetricLabelMethod:    req.Action,
		MetricLabelErrorCode: hdr.ErrorCode,
	}
	duration := o.clock.Now().Sub(o.requestStart)
	o.metrics.apiRequestDuration.With(labels).Observe(duration.Seconds())

	// The following is obsolete and should be removed for 2.6 release
	o.metrics.deprecatedAPIRequestDuration.With(labels).Observe(duration.Seconds())
	o.metrics.deprecatedAPIRequestsTotal.With(labels).Inc()
}
