// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package traceconfigupdater

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/tracer"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
)

// TracingAPI represents the API calls the worker makes.
type TracingAPI interface {
	// GetControllerTracingConfig returns the controller-wide tracing
	// configuration for the supplied agent.
	GetControllerTracingConfig(ctx context.Context, agentTag names.Tag) (tracer.ControllerTracingConfig, error)

	// WatchControllerTracingConfig returns a watcher for controller-wide
	// tracing configuration changes.
	WatchControllerTracingConfig(ctx context.Context, agentTag names.Tag) (watcher.NotifyWatcher, error)
}

// WorkerConfig contains the information required by the worker.
type WorkerConfig struct {
	Agent              agent.Agent
	API                TracingAPI
	AgentConfigChanged *voyeur.Value
	Logger             corelogger.Logger
}

// Validate ensures all the necessary fields have values.
func (c WorkerConfig) Validate() error {
	if c.Agent == nil {
		return errors.NotValidf("missing agent")
	}
	if c.API == nil {
		return errors.NotValidf("missing api")
	}
	if c.AgentConfigChanged == nil {
		return errors.NotValidf("nil AgentConfigChanged")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

type traceConfigUpdater struct {
	config WorkerConfig
	tag    names.Tag
}

// NewWorker returns a worker that keeps the local agent config in sync with the
// controller-wide tracing (OpenTelemetry) configuration.
func NewWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	currentConfig := config.Agent.CurrentConfig()
	w := &traceConfigUpdater{
		config: config,
		tag:    currentConfig.Tag(),
	}
	return watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: w,
	})
}

// SetUp implements watcher.NotifyHandler.
func (w *traceConfigUpdater) SetUp(ctx context.Context) (watcher.NotifyWatcher, error) {
	w.config.Logger.Infof(ctx, "trace config updater worker started")
	if err := w.update(ctx); err != nil {
		return nil, errors.Trace(err)
	}
	return w.config.API.WatchControllerTracingConfig(ctx, w.tag)
}

// Handle implements watcher.NotifyHandler.
func (w *traceConfigUpdater) Handle(ctx context.Context) error {
	return errors.Trace(w.update(ctx))
}

// TearDown implements watcher.NotifyHandler.
func (w *traceConfigUpdater) TearDown() error {
	w.config.Logger.Infof(context.Background(), "trace config updater worker stopped")
	return nil
}

func (w *traceConfigUpdater) update(ctx context.Context) error {
	tracingConfig, err := w.config.API.GetControllerTracingConfig(ctx, w.tag)
	if err != nil {
		return errors.Annotate(err, "getting controller tracing config")
	}

	// Resolve the controller-side config (which uses pointer fields where
	// nil means "default") into the concrete values that belong on the
	// agent config.
	desired := resolveTracingConfig(tracingConfig)

	currentConfig := w.config.Agent.CurrentConfig()
	if currentConfig.OpenTelemetryEnabled() == desired.enabled &&
		currentConfig.OpenTelemetryHTTPEndpoint() == desired.httpEndpoint &&
		currentConfig.OpenTelemetryGRPCEndpoint() == desired.grpcEndpoint &&
		currentConfig.OpenTelemetryCACertificate() == desired.caCertificate &&
		currentConfig.OpenTelemetryInsecure() == desired.insecure &&
		currentConfig.OpenTelemetryStackTraces() == desired.stackTraces &&
		currentConfig.OpenTelemetrySampleRatio() == desired.sampleRatio &&
		currentConfig.OpenTelemetryTailSamplingThreshold() == desired.tailSamplingThreshold {
		return nil
	}

	w.config.Logger.Debugf(ctx, "updating agent tracing config: http=%q grpc=%q",
		desired.httpEndpoint, desired.grpcEndpoint)

	err = w.config.Agent.ChangeConfig(func(setter agent.ConfigSetter) error {
		setter.SetOpenTelemetryEnabled(desired.enabled)
		setter.SetOpenTelemetryHTTPEndpoint(desired.httpEndpoint)
		setter.SetOpenTelemetryGRPCEndpoint(desired.grpcEndpoint)
		setter.SetOpenTelemetryCACertificate(desired.caCertificate)
		setter.SetOpenTelemetryInsecure(desired.insecure)
		setter.SetOpenTelemetryStackTraces(desired.stackTraces)
		setter.SetOpenTelemetrySampleRatio(desired.sampleRatio)
		setter.SetOpenTelemetryTailSamplingThreshold(desired.tailSamplingThreshold)
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "updating agent tracing config")
	}
	w.config.AgentConfigChanged.Set(true)
	return nil
}

// resolvedTracingConfig holds the concrete (non-pointer) values resolved from
// a ControllerTracingConfig, using agent defaults for nil pointer fields.
type resolvedTracingConfig struct {
	enabled               bool
	httpEndpoint          string
	grpcEndpoint          string
	caCertificate         string
	insecure              bool
	stackTraces           bool
	sampleRatio           float64
	tailSamplingThreshold time.Duration
}

// resolveTracingConfig converts the controller-side tracing config (which uses
// pointer fields where nil means "default") into the concrete values that
// should be written to the agent config.
func resolveTracingConfig(cfg tracer.ControllerTracingConfig) resolvedTracingConfig {
	r := resolvedTracingConfig{
		httpEndpoint:          cfg.HTTPEndpoint,
		grpcEndpoint:          cfg.GRPCEndpoint,
		caCertificate:         cfg.CACert,
		insecure:              agent.DefaultOpenTelemetryInsecure,
		stackTraces:           agent.DefaultOpenTelemetryStackTraces,
		sampleRatio:           agent.DefaultOpenTelemetrySampleRatio,
		tailSamplingThreshold: agent.DefaultOpenTelemetryTailSamplingThreshold,
	}
	// Tracing is enabled when at least one endpoint is configured.
	r.enabled = r.httpEndpoint != "" || r.grpcEndpoint != ""

	if cfg.InsecureSkipVerify != nil {
		r.insecure = *cfg.InsecureSkipVerify
	}
	if cfg.StackTraces != nil {
		r.stackTraces = *cfg.StackTraces
	}
	if cfg.SampleRatio != nil {
		r.sampleRatio = *cfg.SampleRatio
	}
	if cfg.TailSamplingThreshold != nil && *cfg.TailSamplingThreshold != "" {
		if d, err := time.ParseDuration(*cfg.TailSamplingThreshold); err == nil {
			r.tailSamplingThreshold = d
		}
	}
	return r
}
