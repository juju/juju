// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/rpc"
)

const (
	facadeLabel    = "facade"
	versionLabel   = "version"
	methodLabel    = "method"
	errorCodeLabel = "error_code"
)

var metricLabelNames = []string{
	facadeLabel,
	versionLabel,
	methodLabel,
	errorCodeLabel,
}

// Config contains the configuration for an Observer.
type Config struct {
	// Clock is the clock to use for all time-related operations.
	Clock clock.Clock

	// PrometheusRegisterer is the prometheus.Registerer in which metric
	// collectors will be registered.
	PrometheusRegisterer prometheus.Registerer
}

// Validate validates the observer factory configuration.
func (cfg Config) Validate() error {
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
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

	apiRequestsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "juju",
		Subsystem: "api",
		Name:      "requests_total",
		Help:      "Number of Juju API requests served.",
	}, metricLabelNames)

	apiRequestDuration := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "juju",
		Subsystem: "api",
		Name:      "request_duration_seconds",
		Help:      "Latency of Juju API requests in seconds.",
	}, metricLabelNames)

	config.PrometheusRegisterer.Unregister(apiRequestsTotal)
	if err := config.PrometheusRegisterer.Register(apiRequestsTotal); err != nil {
		return nil, errors.Trace(err)
	}

	config.PrometheusRegisterer.Unregister(apiRequestDuration)
	if err := config.PrometheusRegisterer.Register(apiRequestDuration); err != nil {
		return nil, errors.Trace(err)
	}

	// Observer is currently stateless, so we return the same one for each
	// API connection. Individual RPC requests still get their own RPC
	// observers.
	o := &Observer{
		clock: config.Clock,
		metrics: metrics{
			apiRequestDuration: apiRequestDuration,
			apiRequestsTotal:   apiRequestsTotal,
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
	apiRequestDuration *prometheus.SummaryVec
	apiRequestsTotal   *prometheus.CounterVec
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
		facadeLabel:    req.Type,
		versionLabel:   strconv.Itoa(req.Version),
		methodLabel:    req.Action,
		errorCodeLabel: hdr.ErrorCode,
	}
	duration := o.clock.Now().Sub(o.requestStart)
	o.metrics.apiRequestDuration.With(labels).Observe(duration.Seconds())
	o.metrics.apiRequestsTotal.With(labels).Inc()
}
