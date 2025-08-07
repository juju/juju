// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater

import (
	"context"
	"strings"
	"time"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	jworker "github.com/juju/juju/internal/worker"
)

// ControllerConfigService defines the methods required to get the controller
// configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(context.Context) (controller.Config, error)

	// WatchControllerConfig watches the controller config for changes.
	WatchControllerConfig(context.Context) (watcher.StringsWatcher, error)
}

// WorkerConfig contains the information necessary to run
// the agent config updater worker.
type WorkerConfig struct {
	Agent                              coreagent.Agent
	ControllerConfigService            ControllerConfigService
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
		return errors.Errorf("missing agent %w", coreerrors.NotValid)
	}
	if c.ControllerConfigService == nil {
		return errors.Errorf("missing ControllerConfigService %w", coreerrors.NotValid)
	}
	if c.Logger == nil {
		return errors.Errorf("missing logger %w", coreerrors.NotValid)
	}
	return nil
}

type agentConfigUpdater struct {
	catacomb catacomb.Catacomb

	config WorkerConfig

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
		return nil, errors.Capture(err)
	}

	w := &agentConfigUpdater{
		config:                             config,
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
		Name: "agent-config-updater",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Capture(err)
	}
	return w, nil
}

func (w *agentConfigUpdater) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	watcher, err := w.config.ControllerConfigService.WatchControllerConfig(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Capture(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-watcher.Changes():
			if err := w.handleConfigChange(ctx); err != nil {
				return errors.Capture(err)
			}
		}
	}
}

func (w *agentConfigUpdater) handleConfigChange(ctx context.Context) error {
	config, err := w.config.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Capture(err)
	}

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

	changeDetected := queryTracingEnabledChanged ||
		queryTracingThresholdChanged ||
		openTelemetryEnabledChanged ||
		openTelemetryEndpointChanged ||
		openTelemetryInsecureChanged ||
		openTelemetryStackTracesChanged ||
		openTelemetrySampleRatioChanged ||
		openTelemetryTailSamplingThresholdChanged

	// If any changes are detected, we need to update the agent config.
	if !changeDetected {
		// Nothing to do, all good.
		return nil
	}

	err = w.config.Agent.ChangeConfig(func(setter coreagent.ConfigSetter) error {
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
		return nil
	})
	if err != nil {
		return errors.Errorf("%w: failed to update agent config", err)
	}

	reason := []string{}
	if queryTracingEnabledChanged {
		reason = append(reason, controller.QueryTracingEnabled)
	}
	if queryTracingThresholdChanged {
		reason = append(reason, controller.QueryTracingThreshold)
	}
	if openTelemetryEnabledChanged {
		reason = append(reason, controller.OpenTelemetryEnabled)
	}
	if openTelemetryEndpointChanged {
		reason = append(reason, controller.OpenTelemetryEndpoint)
	}
	if openTelemetryInsecureChanged {
		reason = append(reason, controller.OpenTelemetryInsecure)
	}
	if openTelemetryStackTracesChanged {
		reason = append(reason, controller.OpenTelemetryStackTraces)
	}
	if openTelemetrySampleRatioChanged {
		reason = append(reason, controller.OpenTelemetrySampleRatio)
	}
	if openTelemetryTailSamplingThresholdChanged {
		reason = append(reason, controller.OpenTelemetryTailSamplingThreshold)
	}

	return errors.Errorf("%w: controller config changed: %s",
		jworker.ErrRestartAgent, strings.Join(reason, ", "))
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
