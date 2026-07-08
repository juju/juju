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

// ControllerBackendFuncFactory returns a backend constructor for the supplied
// controller-local resources.
type ControllerBackendFuncFactory func(
	corelogger.LogSink,
	loki.HTTPClient,
	clock.Clock,
	prometheus.Registerer,
) BackendFunc

// ManifoldConfig defines the names of the manifolds used by logrouter.
type ManifoldConfig struct {
	// AgentName is the dependency name for the agent manifold. When set,
	// the manifold reads the current agent config to create a
	// LokiConfigProvider. Use AgentName for non-controller paths
	// where the agent is available through the dependency graph.
	AgentName string

	APICallerName  string
	HTTPClientName string

	// LokiConfigProvider is a direct LokiConfigProvider. When set, it
	// takes precedence over AgentName. Use this for controller paths
	// where the provider is injected directly.
	LokiConfigProvider LokiConfigProvider

	LogSource            logsender.LogRecordCh
	AgentConfigChanged   *voyeur.Value
	Logger               corelogger.Logger
	Clock                clock.Clock
	PrometheusRegisterer prometheus.Registerer
	DrainOnly            bool

	NewBackendFunc BackendFuncFactory

	RemoveLegacyLogSinkWriter func()
	AddLegacyLogSinkWriter    func() error
}

// Validate validates the manifold configuration.
func (c ManifoldConfig) Validate() error {
	if c.AgentName == "" && c.LokiConfigProvider == nil {
		return errors.NotValidf("empty AgentName and nil LokiConfigProvider")
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
	if c.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
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
	inputs := []string{config.APICallerName, config.HTTPClientName}
	if config.AgentName != "" {
		inputs = append([]string{config.AgentName}, inputs...)
	}

	return dependency.Manifold{
		Inputs: inputs,
		Output: outputFunc,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, internalerrors.Capture(err)
			}

			lokiConfigProvider := config.LokiConfigProvider
			if lokiConfigProvider == nil {
				var a agent.Agent
				if err := getter.Get(config.AgentName, &a); err != nil {
					return nil, err
				}
				lokiConfigProvider = agentLokiConfigProvider{agent: a}
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
				LokiConfigProvider:        lokiConfigProvider,
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

// agentLokiConfigProvider wraps an agent.Agent and implements
// LokiConfigProvider by reading from the current agent config.
type agentLokiConfigProvider struct {
	agent agent.Agent
}

func (p agentLokiConfigProvider) CurrentLokiConfig() (ConfigSnapshot, error) {
	cfg := p.agent.CurrentConfig()
	return ConfigSnapshot{
		Endpoint:           cfg.LokiEndpoint(),
		CACertificate:      cfg.LokiCACert(),
		InsecureSkipVerify: cfg.LokiInsecureSkipVerify(),
		ControllerUUID:     cfg.Controller().Id(),
		ModelUUID:          cfg.Model().Id(),
		AgentID:            cfg.Tag().String(),
		OrgID:              cfg.LokiOrgID(),
	}, nil
}

// ControllerManifoldConfig defines the names of the manifolds used by the
// controller-local logrouter path.
type ControllerManifoldConfig struct {
	HTTPClientName       string
	LokiConfigProvider   LokiConfigProvider
	AgentConfigChanged   *voyeur.Value
	Logger               corelogger.Logger
	Clock                clock.Clock
	PrometheusRegisterer prometheus.Registerer
	LocalLogSink         corelogger.LogSink

	NewBackendFunc ControllerBackendFuncFactory
}

// Validate validates the controller-local manifold configuration.
func (c ControllerManifoldConfig) Validate() error {
	if c.HTTPClientName == "" {
		return errors.NotValidf("empty HTTPClientName")
	}
	if c.LokiConfigProvider == nil {
		return errors.NotValidf("nil LokiConfigProvider")
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
	if c.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if c.LocalLogSink == nil {
		return errors.NotValidf("nil LocalLogSink")
	}
	if c.NewBackendFunc == nil {
		return errors.NotValidf("nil NewBackendFunc")
	}
	return nil
}

// ControllerManifold returns a dependency manifold that runs a controller-only
// logrouter worker. The controller path uses the local logsink directly in
// logsink mode so it can switch model loggers without depending on api-caller.
func ControllerManifold(config ControllerManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.HTTPClientName,
		},
		Output: outputFunc,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, internalerrors.Capture(err)
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
				LokiConfigProvider:        config.LokiConfigProvider,
				LogSource:                 make(logsender.LogRecordCh),
				AgentConfigChanged:        config.AgentConfigChanged,
				Logger:                    config.Logger,
				Clock:                     config.Clock,
				ConvergeTimeout:           defaultConvergeTimeout,
				RestartDelay:              defaultRestartDelay,
				NewBackend:                config.NewBackendFunc(config.LocalLogSink, httpClient, config.Clock, config.PrometheusRegisterer),
				RemoveLegacyLogSinkWriter: func() {},
				AddLegacyLogSinkWriter:    func() error { return nil },
			})
		},
	}
}

// outputFunc extracts the LogRouter interface from the running log router
// worker so that dependants (such as the log sink) can access the active
// LogSink.
func outputFunc(in worker.Worker, out any) error {
	if w, ok := in.(*logRouter); ok {
		switch p := out.(type) {
		case *LogRouter:
			*p = w
		case *corelogger.LogSink:
			*p = w.LogSink()
		default:
			return errors.Errorf("unsupported output type %T", out)
		}
		return nil
	}
	return errors.Errorf("expected *logRouter, got %T", in)
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
			lokiConfig.OrgID = snapshot.OrgID

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

// NewControllerBackend returns a backend constructor for controller-local
// model log delivery. In logsink mode it writes to the local logsink directly
// instead of sending to the api logsink endpoint.
func NewControllerBackend(
	localLogSink corelogger.LogSink,
	httpClient loki.HTTPClient,
	clock clock.Clock,
	registerer prometheus.Registerer,
) BackendFunc {
	return func(backendType BackendType, snapshot ConfigSnapshot) (Backend, error) {
		switch backendType {
		case BackendTypeLogSink:
			return backends.NewLocal(backends.LocalConfig{
				BackendBufferSize: defaultBackendBufferSize,
				LogSink:           localLogSink,
			})

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
			lokiConfig.OrgID = snapshot.OrgID

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
