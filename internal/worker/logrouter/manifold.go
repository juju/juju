// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logrouter

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apilogsender "github.com/juju/juju/api/logsender"
	corelogger "github.com/juju/juju/core/logger"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/loki"
	"github.com/juju/juju/internal/worker/logrouter/backends"
	"github.com/juju/juju/internal/worker/logsender"
)

// BackendFuncFactory returns a backend constructor for the supplied agent
// resources.
type BackendFuncFactory func(
	base.APICaller,
	loki.HTTPClient,
	clock.Clock,
) BackendFunc

// ManifoldConfig defines the names of the manifolds used by logrouter.
type ManifoldConfig struct {
	AgentName          string
	APICallerName      string
	LogSource          logsender.LogRecordCh
	AgentConfigChanged *voyeur.Value
	Logger             corelogger.Logger
	Clock              clock.Clock
	HTTPClient         loki.HTTPClient
	DrainOnly          bool

	NewBackendFunc BackendFuncFactory
}

// Validate validates the manifold configuration.
func (c ManifoldConfig) Validate() error {
	if c.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if c.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if c.LogSource == nil {
		return errors.NotValidf("nil LogSource")
	}
	if c.AgentConfigChanged == nil {
		return errors.NotValidf("nil AgentConfigChanged")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.NewBackendFunc == nil {
		return errors.NotValidf("nil NewBackendFunc")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the logrouter worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.AgentName, config.APICallerName},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, internalerrors.Capture(err)
			}

			var a agent.Agent
			if err := getter.Get(config.AgentName, &a); err != nil {
				return nil, err
			}
			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}

			return NewWorker(WorkerConfig{
				Agent:              a,
				LogSource:          config.LogSource,
				AgentConfigChanged: config.AgentConfigChanged,
				Logger:             config.Logger,
				Clock:              config.Clock,
				DrainOnly:          config.DrainOnly,
				ConvergeTimeout:    defaultConvergeTimeout,
				RestartDelay:       defaultRestartDelay,
				NewBackend:         config.NewBackendFunc(apiCaller, config.HTTPClient, config.Clock),
			})
		},
	}
}

// NewBackend returns the default backend constructor.
func NewBackend(
	apiCaller base.APICaller,
	httpClient loki.HTTPClient,
	clock clock.Clock,
) BackendFunc {
	return func(backendType BackendType, snapshot ConfigSnapshot) (Backend, error) {
		switch backendType {
		case BackendTypeLogSink:
			logSenderAPI := apilogsender.NewAPI(apiCaller)
			return backends.NewLogSink(logSenderAPI, defaultBackendBufferSize)

		case BackendTypeLoki:
			lokiConfig := loki.DefaultConfig()
			lokiConfig.HTTPClient = httpClient
			lokiConfig.Clock = clock

			return backends.NewLoki(backends.LokiConfig{
				BackendBufferSize: defaultBackendBufferSize,
				ClientConfig:      lokiConfig,
				Endpoint:          snapshot.Endpoint,
				ControllerUUID:    snapshot.ControllerUUID,
				ModelUUID:         snapshot.ModelUUID,
				AgentID:           snapshot.AgentID,
				NewClient: func(endpoint string, cfg loki.Config) (backends.LokiClient, error) {
					return loki.NewClient(endpoint, cfg)
				},
			})

		case BackendTypeDrain:
			return backends.NewDrain(defaultBackendBufferSize)

		default:
			return nil, errors.NotValidf("unknown logrouter backend type %q", backendType)
		}
	}
}
