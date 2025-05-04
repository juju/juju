// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	jworker "github.com/juju/juju/internal/worker"
)

// ControllerConfigService defines the methods required to get the controller
// configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(context.Context) (controller.Config, error)

	// WatchControllerConfig watches the controller config for changes.
	WatchControllerConfig() (watcher.StringsWatcher, error)
}

// WorkerConfig contains the information necessary to run
// the agent config updater worker.
type WorkerConfig struct {
	Agent                              coreagent.Agent
	ControllerConfigService            ControllerConfigService
	JujuDBSnapChannel                  string
	QueryTracingEnabled                bool
	QueryTracingThreshold              time.Duration
	OpenTelemetryEnabled               bool
	OpenTelemetryEndpoint              string
	OpenTelemetryInsecure              bool
	OpenTelemetryStackTraces           bool
	OpenTelemetrySampleRatio           float64
	OpenTelemetryTailSamplingThreshold time.Duration
	ObjectStoreType                    objectstore.BackendType
	Logger                             logger.Logger
}

// Validate ensures that the required values are set in the structure.
func (c *WorkerConfig) Validate() error {
	if c.Agent == nil {
		return errors.NotValidf("missing agent")
	}
	if c.ControllerConfigService == nil {
		return errors.NotValidf("missing ControllerConfigService")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

type agentConfigUpdater struct {
	catacomb catacomb.Catacomb

	config WorkerConfig

	jujuDBSnapChannel                  string
	queryTracingEnabled                bool
	queryTracingThreshold              time.Duration
	openTelemetryEnabled               bool
	openTelemetryEndpoint              string
	openTelemetryInsecure              bool
	openTelemetryStackTraces           bool
	openTelemetrySampleRatio           float64
	openTelemetryTailSamplingThreshold time.Duration
	objectStoreType                    objectstore.BackendType
}

// NewWorker creates a new agent config updater worker.
func NewWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &agentConfigUpdater{
		config:                             config,
		jujuDBSnapChannel:                  config.JujuDBSnapChannel,
		queryTracingEnabled:                config.QueryTracingEnabled,
		queryTracingThreshold:              config.QueryTracingThreshold,
		openTelemetryEnabled:               config.OpenTelemetryEnabled,
		openTelemetryEndpoint:              config.OpenTelemetryEndpoint,
		openTelemetryInsecure:              config.OpenTelemetryInsecure,
		openTelemetryStackTraces:           config.OpenTelemetryStackTraces,
		openTelemetrySampleRatio:           config.OpenTelemetrySampleRatio,
		openTelemetryTailSamplingThreshold: config.OpenTelemetryTailSamplingThreshold,
		objectStoreType:                    config.ObjectStoreType,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (w *agentConfigUpdater) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	watcher, err := w.config.ControllerConfigService.WatchControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-watcher.Changes():
			if err := w.handleConfigChange(ctx); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (w *agentConfigUpdater) handleConfigChange(ctx context.Context) error {
	config, err := w.config.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	jujuDBSnapChannel := config.JujuDBSnapChannel()
	jujuDBSnapChannelChanged := jujuDBSnapChannel != w.jujuDBSnapChannel

	queryTracingEnabled := config.QueryTracingEnabled()
	queryTracingEnabledChanged := queryTracingEnabled != w.queryTracingEnabled

	queryTracingThreshold := config.QueryTracingThreshold()
	queryTracingThresholdChanged := queryTracingThreshold != w.queryTracingThreshold

	openTelemetryEnabled := config.OpenTelemetryEnabled()
	openTelemetryEnabledChanged := openTelemetryEnabled != w.openTelemetryEnabled

	openTelemetryEndpoint := config.OpenTelemetryEndpoint()
	openTelemetryEndpointChanged := openTelemetryEndpoint != w.openTelemetryEndpoint

	openTelemetryInsecure := config.OpenTelemetryInsecure()
	openTelemetryInsecureChanged := openTelemetryInsecure != w.openTelemetryInsecure

	openTelemetryStackTraces := config.OpenTelemetryStackTraces()
	openTelemetryStackTracesChanged := openTelemetryStackTraces != w.openTelemetryStackTraces

	openTelemetrySampleRatio := config.OpenTelemetrySampleRatio()
	openTelemetrySampleRatioChanged := openTelemetrySampleRatio != w.openTelemetrySampleRatio

	openTelemetryTailSamplingThreshold := config.OpenTelemetryTailSamplingThreshold()
	openTelemetryTailSamplingThresholdChanged := openTelemetryTailSamplingThreshold != w.openTelemetryTailSamplingThreshold

	objectStoreType := config.ObjectStoreType()
	objectStoreTypeChanged := objectStoreType != w.objectStoreType

	changeDetected := jujuDBSnapChannelChanged ||
		queryTracingEnabledChanged ||
		queryTracingThresholdChanged ||
		openTelemetryEnabledChanged ||
		openTelemetryEndpointChanged ||
		openTelemetryInsecureChanged ||
		openTelemetryStackTracesChanged ||
		openTelemetrySampleRatioChanged ||
		openTelemetryTailSamplingThresholdChanged ||
		objectStoreTypeChanged

	// If any changes are detected, we need to update the agent config.
	if !changeDetected {
		// Nothing to do, all good.
		return nil
	}

	err = w.config.Agent.ChangeConfig(func(setter coreagent.ConfigSetter) error {
		if jujuDBSnapChannelChanged {
			w.config.Logger.Debugf(ctx, "setting agent config mongo snap channel: %q => %q", w.jujuDBSnapChannel, jujuDBSnapChannel)
			setter.SetJujuDBSnapChannel(jujuDBSnapChannel)
		}
		if queryTracingEnabledChanged {
			w.config.Logger.Debugf(ctx, "setting agent config query tracing enabled: %v => %v", w.queryTracingEnabled, queryTracingEnabled)
			setter.SetQueryTracingEnabled(queryTracingEnabled)
		}
		if queryTracingThresholdChanged {
			w.config.Logger.Debugf(ctx, "setting agent config query tracing threshold: %v => %v", w.queryTracingThreshold, queryTracingThreshold)
			setter.SetQueryTracingThreshold(queryTracingThreshold)
		}
		if openTelemetryEnabledChanged {
			w.config.Logger.Debugf(ctx, "setting agent config open telemetry enabled: %v => %v", w.openTelemetryEnabled, openTelemetryEnabled)
			setter.SetOpenTelemetryEnabled(openTelemetryEnabled)
		}
		if openTelemetryEndpointChanged {
			w.config.Logger.Debugf(ctx, "setting agent config open telemetry endpoint: %v => %v", w.openTelemetryEndpoint, openTelemetryEndpoint)
			setter.SetOpenTelemetryEndpoint(openTelemetryEndpoint)
		}
		if openTelemetryInsecureChanged {
			w.config.Logger.Debugf(ctx, "setting agent config open telemetry insecure: %v => %v", w.openTelemetryInsecure, openTelemetryInsecure)
			setter.SetOpenTelemetryInsecure(openTelemetryInsecure)
		}
		if openTelemetryStackTracesChanged {
			w.config.Logger.Debugf(ctx, "setting agent config open telemetry stack traces: %v => %v", w.openTelemetryStackTraces, openTelemetryStackTraces)
			setter.SetOpenTelemetryStackTraces(openTelemetryStackTraces)
		}
		if openTelemetrySampleRatioChanged {
			w.config.Logger.Debugf(ctx, "setting agent config open telemetry sample ratio: %v => %v", w.openTelemetrySampleRatio, openTelemetrySampleRatio)
			setter.SetOpenTelemetrySampleRatio(openTelemetrySampleRatio)
		}
		if openTelemetryTailSamplingThresholdChanged {
			w.config.Logger.Debugf(ctx, "setting agent config open telemetry tail sampling threshold: %v => %v", w.openTelemetryTailSamplingThreshold, openTelemetryTailSamplingThreshold)
			setter.SetOpenTelemetryTailSamplingThreshold(openTelemetryTailSamplingThreshold)
		}
		if objectStoreTypeChanged {
			w.config.Logger.Debugf(ctx, "setting agent config object store type: %v => %v", w.objectStoreType, objectStoreType)
			setter.SetObjectStoreType(objectStoreType)
		}
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "failed to update agent config")
	}

	// If the object store type is set to "s3" then state that the associated
	// config also needs to be set.
	if objectStoreType == objectstore.S3Backend {
		if err := controller.HasCompleteS3ControllerConfig(config); err != nil {
			w.config.Logger.Warningf(ctx, "object store type is set to s3 but config not set: %v", err)
		}
	}

	return jworker.ErrRestartAgent
}

// Kill implements Worker.Kill().
func (w *agentConfigUpdater) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements Worker.Wait().
func (w *agentConfigUpdater) Wait() error {
	return w.catacomb.Wait()
}

func (w *agentConfigUpdater) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
