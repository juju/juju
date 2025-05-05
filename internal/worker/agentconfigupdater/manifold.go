// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	jujuagent "github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent/agent"
	"github.com/juju/juju/api/base"
	coreagent "github.com/juju/juju/core/agent"
	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/services"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/trace"
)

// ControllerDomainServices is an interface that defines the
// services that are required by the agent config updater.
type ControllerDomainServices interface {
	// ControllerConfig returns the controller configuration service.
	ControllerConfig() ControllerConfigService
	// ControllerNode returns the controller node service.
	ControllerNode() ControllerNodeService
}

// ControllerNodeService is an interface that defines the methods that are
// required to check if a machine or container agent is a controller node.
type ControllerNodeService interface {
	// IsControllerNode returns true if the machine is a controller node.
	IsControllerNode(ctx context.Context, nodeID string) (bool, error)
}

// GetControllerDomainServicesFunc is a function that retrieves the
// controller domain services from the dependency getter.
type GetControllerDomainServicesFunc func(dependency.Getter, string) (ControllerDomainServices, error)

// ManifoldConfig provides the dependencies for the
// agent config updater manifold.
type ManifoldConfig struct {
	AgentName                     string
	DomainServicesName            string
	TraceName                     string
	Logger                        logger.Logger
	GetControllerDomainServicesFn GetControllerDomainServicesFunc

	// TODO (stickupkid): This is only required to know if it's a controller
	// or not. Along with getting the state serving info. This is all available
	// in dqlite already.
	APICallerName string
}

// Manifold defines a simple start function which
// runs after the API connection has come up. If the machine agent is
// a controller, it grabs the state serving info over the API and
// records it to agent configuration, and then stops.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.DomainServicesName,
			config.TraceName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			// Get the agent.
			var agent jujuagent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}

			var (
				logger        = config.Logger
				currentConfig = agent.CurrentConfig()
			)

			// Grab the tag and ensure that it's for a controller.
			tag := currentConfig.Tag()
			if !coreagent.IsAllowedControllerTag(tag.Kind()) {
				return nil, errors.New("agent's tag is not a machine or controller agent tag")
			}

			// Get API connection.
			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}

			controllerServices, err := config.GetControllerDomainServicesFn(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Annotate(err, "getting controller domain services")
			}

			controllerNodeService := controllerServices.ControllerNode()
			if isControllerNode, err := controllerNodeService.IsControllerNode(ctx, tag.Id()); err != nil {
				return nil, errors.Annotate(err, "checking is controller")
			} else if !isControllerNode {
				// Not a controller, nothing to do.
				return nil, dependency.ErrUninstall
			}

			controllerConfigService := controllerServices.ControllerConfig()

			// Get the tracer from the context.
			var tracerGetter trace.TracerGetter
			if err := getter.Get(config.TraceName, &tracerGetter); err != nil {
				return nil, errors.Trace(err)
			}

			tracer, err := tracerGetter.GetTracer(ctx, coretrace.Namespace("agentconfigupdater", currentConfig.Model().Id()))
			if err != nil {
				tracer = coretrace.NoopTracer{}
			}

			controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
			if err != nil {
				return nil, errors.Trace(err)
			}

			agentsJujuDBSnapChannel := currentConfig.JujuDBSnapChannel()
			configJujuDBSnapChannel := controllerConfig.JujuDBSnapChannel()
			jujuDBSnapChannelChanged := agentsJujuDBSnapChannel != configJujuDBSnapChannel

			agentsQueryTracingEnabled := currentConfig.QueryTracingEnabled()
			configQueryTracingEnabled := controllerConfig.QueryTracingEnabled()
			queryTracingEnabledChanged := agentsQueryTracingEnabled != configQueryTracingEnabled

			agentsQueryTracingThreshold := currentConfig.QueryTracingThreshold()
			configQueryTracingThreshold := controllerConfig.QueryTracingThreshold()
			queryTracingThresholdChanged := agentsQueryTracingThreshold != configQueryTracingThreshold

			agentsOpenTelemetryEnabled := currentConfig.OpenTelemetryEnabled()
			configOpenTelemetryEnabled := controllerConfig.OpenTelemetryEnabled()
			openTelemetryEnabledChanged := agentsOpenTelemetryEnabled != configOpenTelemetryEnabled

			agentsOpenTelemetryEndpoint := currentConfig.OpenTelemetryEndpoint()
			configOpenTelemetryEndpoint := controllerConfig.OpenTelemetryEndpoint()
			openTelemetryEndpointChanged := agentsOpenTelemetryEndpoint != configOpenTelemetryEndpoint

			agentsOpenTelemetryInsecure := currentConfig.OpenTelemetryInsecure()
			configOpenTelemetryInsecure := controllerConfig.OpenTelemetryInsecure()
			openTelemetryInsecureChanged := agentsOpenTelemetryInsecure != configOpenTelemetryInsecure

			agentsOpenTelemetryStackTraces := currentConfig.OpenTelemetryStackTraces()
			configOpenTelemetryStackTraces := controllerConfig.OpenTelemetryStackTraces()
			openTelemetryStackTracesChanged := agentsOpenTelemetryStackTraces != configOpenTelemetryStackTraces

			agentsOpenTelemetrySampleRatio := currentConfig.OpenTelemetrySampleRatio()
			configOpenTelemetrySampleRatio := controllerConfig.OpenTelemetrySampleRatio()
			openTelemetrySampleRatioChanged := agentsOpenTelemetrySampleRatio != configOpenTelemetrySampleRatio

			agentsOpenTelemetryTailSamplingThreshold := currentConfig.OpenTelemetryTailSamplingThreshold()
			configOpenTelemetryTailSamplingThreshold := controllerConfig.OpenTelemetryTailSamplingThreshold()
			openTelemetryTailSamplingThresholdChanged := agentsOpenTelemetryTailSamplingThreshold != configOpenTelemetryTailSamplingThreshold

			agentsObjectStoreType := currentConfig.ObjectStoreType()
			configObjectStoreType := controllerConfig.ObjectStoreType()
			objectStoreTypeChanged := agentsObjectStoreType != configObjectStoreType

			apiState, err := apiagent.NewClient(apiCaller, apiagent.WithTracer(tracer))
			if err != nil {
				return nil, errors.Trace(err)
			}

			// If the machine needs Client, grab the state serving info
			// over the API and write it to the agent configuration.

			// Do the initial state serving info and mongo profile checks before
			// attempting to get the central hub. The central hub is only
			// running when the agent is a controller. If the agent isn't a
			// controller but should be, the agent config will not have any
			// state serving info but the database will think that we should be.
			// In those situations we need to update the local config and
			// restart.
			info, err := apiState.StateServingInfo(ctx)
			if err != nil {
				return nil, errors.Annotate(err, "getting state serving info")
			}
			err = agent.ChangeConfig(func(config jujuagent.ConfigSetter) error {
				existing, hasInfo := config.StateServingInfo()
				if hasInfo {
					// Use the existing cert and key as they appear to have been
					// already updated by the cert updater worker to have this
					// machine's IP address as part of the cert. This changed
					// cert is never put back into the database, so it isn't
					// reflected in the copy we have got from apiState.
					info.Cert = existing.Cert
					info.PrivateKey = existing.PrivateKey
				}
				config.SetStateServingInfo(info)
				if jujuDBSnapChannelChanged {
					logger.Debugf(ctx, "setting agent config mongo snap channel: %q => %q", agentsJujuDBSnapChannel, configJujuDBSnapChannel)
					config.SetJujuDBSnapChannel(configJujuDBSnapChannel)
				}
				if queryTracingEnabledChanged {
					logger.Debugf(ctx, "setting agent config query tracing enabled: %t => %t", agentsQueryTracingEnabled, configQueryTracingEnabled)
					config.SetQueryTracingEnabled(configQueryTracingEnabled)
				}
				if queryTracingThresholdChanged {
					logger.Debugf(ctx, "setting agent config query tracing threshold: %d => %d", agentsQueryTracingThreshold, configQueryTracingThreshold)
					config.SetQueryTracingThreshold(configQueryTracingThreshold)
				}
				if openTelemetryEnabledChanged {
					logger.Debugf(ctx, "setting open telemetry enabled: %t => %t", agentsOpenTelemetryEnabled, configOpenTelemetryEnabled)
					config.SetOpenTelemetryEnabled(configOpenTelemetryEnabled)
				}
				if openTelemetryEndpointChanged {
					logger.Debugf(ctx, "setting open telemetry endpoint: %q => %q", agentsOpenTelemetryEndpoint, configOpenTelemetryEndpoint)
					config.SetOpenTelemetryEndpoint(configOpenTelemetryEndpoint)
				}
				if openTelemetryInsecureChanged {
					logger.Debugf(ctx, "setting open telemetry insecure: %t => %t", agentsOpenTelemetryInsecure, configOpenTelemetryInsecure)
					config.SetOpenTelemetryInsecure(configOpenTelemetryInsecure)
				}
				if openTelemetryStackTracesChanged {
					logger.Debugf(ctx, "setting open telemetry stack trace: %t => %t", agentsOpenTelemetryStackTraces, configOpenTelemetryStackTraces)
					config.SetOpenTelemetryStackTraces(configOpenTelemetryStackTraces)
				}
				if openTelemetrySampleRatioChanged {
					logger.Debugf(ctx, "setting open telemetry sample ratio: %f => %f", agentsOpenTelemetrySampleRatio, configOpenTelemetrySampleRatio)
					config.SetOpenTelemetrySampleRatio(configOpenTelemetrySampleRatio)
				}
				if openTelemetryTailSamplingThresholdChanged {
					logger.Debugf(ctx, "setting open telemetry tail sampling threshold: %f => %f", agentsOpenTelemetryTailSamplingThreshold, configOpenTelemetryTailSamplingThreshold)
					config.SetOpenTelemetryTailSamplingThreshold(configOpenTelemetryTailSamplingThreshold)
				}
				if objectStoreTypeChanged {
					logger.Debugf(ctx, "setting object store type: %q => %q", agentsObjectStoreType, configObjectStoreType)
					config.SetObjectStoreType(configObjectStoreType)
				}

				return nil
			})
			if err != nil {
				return nil, errors.Trace(err)
			}

			// If we need a restart, return the fatal error.
			if jujuDBSnapChannelChanged {
				logger.Infof(ctx, "restarting agent for new mongo snap channel")
				return nil, jworker.ErrRestartAgent
			} else if queryTracingEnabledChanged {
				logger.Infof(ctx, "restarting agent for new query tracing enabled")
				return nil, jworker.ErrRestartAgent
			} else if queryTracingThresholdChanged {
				logger.Infof(ctx, "restarting agent for new query tracing threshold")
				return nil, jworker.ErrRestartAgent
			} else if openTelemetryEnabledChanged {
				logger.Infof(ctx, "restarting agent for new open telemetry enabled")
				return nil, jworker.ErrRestartAgent
			} else if openTelemetryEndpointChanged {
				logger.Infof(ctx, "restarting agent for new open telemetry endpoint")
				return nil, jworker.ErrRestartAgent
			} else if openTelemetryInsecureChanged {
				logger.Infof(ctx, "restarting agent for new open telemetry insecure")
				return nil, jworker.ErrRestartAgent
			} else if openTelemetryStackTracesChanged {
				logger.Infof(ctx, "restarting agent for new open telemetry stack traces")
				return nil, jworker.ErrRestartAgent
			} else if openTelemetrySampleRatioChanged {
				logger.Infof(ctx, "restarting agent for new open telemetry sample ratio")
				return nil, jworker.ErrRestartAgent
			} else if openTelemetryTailSamplingThresholdChanged {
				logger.Infof(ctx, "restarting agent for new open telemetry tail sampling threshold")
				return nil, jworker.ErrRestartAgent
			} else if objectStoreTypeChanged {
				logger.Infof(ctx, "restarting agent for new object store type")
				return nil, jworker.ErrRestartAgent
			}

			return NewWorker(WorkerConfig{
				Agent:                              agent,
				ControllerConfigService:            controllerConfigService,
				JujuDBSnapChannel:                  configJujuDBSnapChannel,
				QueryTracingEnabled:                configQueryTracingEnabled,
				QueryTracingThreshold:              configQueryTracingThreshold,
				OpenTelemetryEnabled:               configOpenTelemetryEnabled,
				OpenTelemetryEndpoint:              configOpenTelemetryEndpoint,
				OpenTelemetryInsecure:              configOpenTelemetryInsecure,
				OpenTelemetryStackTraces:           configOpenTelemetryStackTraces,
				OpenTelemetrySampleRatio:           configOpenTelemetrySampleRatio,
				OpenTelemetryTailSamplingThreshold: configOpenTelemetryTailSamplingThreshold,
				ObjectStoreType:                    configObjectStoreType,
				Logger:                             config.Logger,
			})
		},
	}
}

// GetControllerDomainServices retrieves the controller domain services
// from the dependency getter.
func GetControllerDomainServices(getter dependency.Getter, name string) (ControllerDomainServices, error) {
	return coredependency.GetDependencyByName(getter, name, func(s services.ControllerDomainServices) ControllerDomainServices {
		return controllerDomainServices{
			ControllerConfigService: s.ControllerConfig(),
			ControllerNodeService:   s.ControllerNode(),
		}
	})
}

type controllerDomainServices struct {
	ControllerConfigService
	ControllerNodeService
}

// ControllerConfigService is an interface that defines the methods that are
// required to get the controller configuration.
func (s controllerDomainServices) ControllerConfig() ControllerConfigService {
	return s.ControllerConfigService
}

// ControllerNodeService is an interface that defines the methods that are
// required to check if a machine or container agent is a controller node.
func (s controllerDomainServices) ControllerNode() ControllerNodeService {
	return s.ControllerNodeService
}
