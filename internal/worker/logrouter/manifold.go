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
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apilogsender "github.com/juju/juju/api/logsender"
	corehttp "github.com/juju/juju/core/http"
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
	prometheus.Registerer,
) BackendFunc

// ManifoldConfig defines the names of the manifolds used by logrouter.
type ManifoldConfig struct {
	AgentName            string
	APICallerName        string
	HTTPClientName       string
	LogSource            logsender.LogRecordCh
	AgentConfigChanged   *voyeur.Value
	Logger               corelogger.Logger
	Clock                clock.Clock
	PrometheusRegisterer prometheus.Registerer
	DrainOnly            bool

	NewBackendFunc BackendFuncFactory

	// RemoveLegacyLogSinkWriter is called when switching to Loki
	// backend mode. It must be idempotent.
	RemoveLegacyLogSinkWriter func()

	// AddLegacyLogSinkWriter is called when switching to LogSink
	// backend mode. It must be idempotent.
	AddLegacyLogSinkWriter func() error
}

// Validate validates the manifold configuration.
func (c ManifoldConfig) Validate() error {
	if c.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if c.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if c.HTTPClientName == "" {
		return errors.NotValidf("empty HTTPClientName")
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
	if c.RemoveLegacyLogSinkWriter == nil {
		return errors.NotValidf("nil RemoveLegacyLogSinkWriter")
	}
	if c.AddLegacyLogSinkWriter == nil {
		return errors.NotValidf("nil AddLegacyLogSinkWriter")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the logrouter worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.AgentName, config.APICallerName, config.HTTPClientName},
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
			var httpClientGetter corehttp.HTTPClientGetter
			if err := getter.Get(config.HTTPClientName, &httpClientGetter); err != nil {
				return nil, err
			}
			httpClient, err := httpClientGetter.GetHTTPClient(ctx, corehttp.LokiPurpose)
			if err != nil {
				return nil, internalerrors.Capture(err)
			}

			return NewWorker(WorkerConfig{
				Agent:                     a,
				LogSource:                 config.LogSource,
				AgentConfigChanged:        config.AgentConfigChanged,
				Logger:                    config.Logger,
				Clock:                     config.Clock,
				DrainOnly:                 config.DrainOnly,
				ConvergeTimeout:           defaultConvergeTimeout,
				RestartDelay:              defaultRestartDelay,
				NewBackend:                config.NewBackendFunc(apiCaller, httpClient, config.Clock, config.PrometheusRegisterer),
				RemoveLegacyLogSinkWriter: config.RemoveLegacyLogSinkWriter,
				AddLegacyLogSinkWriter:    config.AddLegacyLogSinkWriter,
			})
		},
	}
}

// NewBackend returns the default backend constructor.
func NewBackend(
	apiCaller base.APICaller,
	httpClient loki.HTTPClient,
	clock clock.Clock,
	registerer prometheus.Registerer,
) BackendFunc {
	return func(backendType BackendType, snapshot ConfigSnapshot) (Backend, error) {
		switch backendType {
		case BackendTypeLogSink:
			logSenderAPI := apilogsender.NewAPI(apiCaller)
			return backends.NewLogSink(logSenderAPI, defaultBackendBufferSize)

		case BackendTypeLoki:
			if updater, ok := httpClient.(corehttp.CACertUpdater); ok {
				insecureSkipVerify := false
				if snapshot.InsecureSkipVerify != nil {
					insecureSkipVerify = *snapshot.InsecureSkipVerify
				}
				if err := updater.ReplaceCACert(snapshot.CACertificate, insecureSkipVerify); err != nil {
					return nil, internalerrors.Capture(err)
				}
			}

			lokiConfig := loki.DefaultConfig()
			lokiConfig.HTTPClient = httpClient
			lokiConfig.Clock = clock

			return backends.NewLoki(backends.LokiConfig{
				BackendBufferSize:    defaultBackendBufferSize,
				ClientConfig:         lokiConfig,
				Endpoint:             snapshot.Endpoint,
				ControllerUUID:       snapshot.ControllerUUID,
				ModelUUID:            snapshot.ModelUUID,
				AgentID:              snapshot.AgentID,
				PrometheusRegisterer: registerer,
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
