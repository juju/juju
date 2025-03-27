// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	controllermsg "github.com/juju/juju/internal/pubsub/controller"
	jworker "github.com/juju/juju/internal/worker"
)

// WorkerConfig contains the information necessary to run
// the agent config updater worker.
type WorkerConfig struct {
	Agent                              coreagent.Agent
	Hub                                *pubsub.StructuredHub
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
	if c.Hub == nil {
		return errors.NotValidf("missing hub")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

type agentConfigUpdater struct {
	config WorkerConfig

	tomb                               tomb.Tomb
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

	started := make(chan struct{})
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
	w.tomb.Go(func() error {
		return w.loop(started)
	})
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		return nil, errors.New("worker failed to start properly")
	}
	return w, nil
}

func (w *agentConfigUpdater) loop(started chan struct{}) error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	unsubscribe, err := w.config.Hub.Subscribe(controllermsg.ConfigChanged, w.onConfigChanged)
	if err != nil {
		w.config.Logger.Criticalf(ctx, "programming error in subscribe function: %v", err)
		return errors.Trace(err)
	}
	defer unsubscribe()
	// Let the caller know we are done.
	close(started)
	// Don't exit until we are told to. Exiting unsubscribes.
	<-w.tomb.Dying()
	w.config.Logger.Tracef(ctx, "agentConfigUpdater loop finished")
	return nil
}

func (w *agentConfigUpdater) onConfigChanged(topic string, data controllermsg.ConfigChangedMessage, err error) {
	ctx, cancel := w.scopedContext()
	defer cancel()

	if err != nil {
		w.config.Logger.Criticalf(ctx, "programming error in %s message data: %v", topic, err)
		return
	}

	jujuDBSnapChannel := data.Config.JujuDBSnapChannel()
	jujuDBSnapChannelChanged := jujuDBSnapChannel != w.jujuDBSnapChannel

	queryTracingEnabled := data.Config.QueryTracingEnabled()
	queryTracingEnabledChanged := queryTracingEnabled != w.queryTracingEnabled

	queryTracingThreshold := data.Config.QueryTracingThreshold()
	queryTracingThresholdChanged := queryTracingThreshold != w.queryTracingThreshold

	openTelemetryEnabled := data.Config.OpenTelemetryEnabled()
	openTelemetryEnabledChanged := openTelemetryEnabled != w.openTelemetryEnabled

	openTelemetryEndpoint := data.Config.OpenTelemetryEndpoint()
	openTelemetryEndpointChanged := openTelemetryEndpoint != w.openTelemetryEndpoint

	openTelemetryInsecure := data.Config.OpenTelemetryInsecure()
	openTelemetryInsecureChanged := openTelemetryInsecure != w.openTelemetryInsecure

	openTelemetryStackTraces := data.Config.OpenTelemetryStackTraces()
	openTelemetryStackTracesChanged := openTelemetryStackTraces != w.openTelemetryStackTraces

	openTelemetrySampleRatio := data.Config.OpenTelemetrySampleRatio()
	openTelemetrySampleRatioChanged := openTelemetrySampleRatio != w.openTelemetrySampleRatio

	openTelemetryTailSamplingThreshold := data.Config.OpenTelemetryTailSamplingThreshold()
	openTelemetryTailSamplingThresholdChanged := openTelemetryTailSamplingThreshold != w.openTelemetryTailSamplingThreshold

	objectStoreType := data.Config.ObjectStoreType()
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
		return
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
		w.tomb.Kill(errors.Annotate(err, "failed to update agent config"))
		return
	}

	// If the object store type is set to "s3" then state that the associated
	// config also needs to be set.
	if objectStoreType == objectstore.S3Backend {
		if err := controller.HasCompleteS3ControllerConfig(data.Config); err != nil {
			w.config.Logger.Warningf(ctx, "object store type is set to s3 but config not set: %v", err)
		}
	}

	w.tomb.Kill(jworker.ErrRestartAgent)
}

// Kill implements Worker.Kill().
func (w *agentConfigUpdater) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements Worker.Wait().
func (w *agentConfigUpdater) Wait() error {
	return w.tomb.Wait()
}

func (w *agentConfigUpdater) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}
