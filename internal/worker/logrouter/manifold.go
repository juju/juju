// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logrouter

import (
	"context"
	"net/http"
	"net/url"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
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
	agent.Agent,
	func(context.Context, *api.Info, api.DialOpts) (api.Connection, error),
	corehttp.HTTPClientGetter,
	clock.Clock,
) BackendFunc

// ManifoldConfig defines the names of the manifolds used by logrouter.
type ManifoldConfig struct {
	AgentName          string
	LogSource          logsender.LogRecordCh
	AgentConfigChanged *voyeur.Value
	Logger             corelogger.Logger
	Clock              clock.Clock
	HTTPClientName     string
	DrainOnly          bool

	NewAPIOpen     func(context.Context, *api.Info, api.DialOpts) (api.Connection, error)
	NewBackendFunc BackendFuncFactory
}

// Validate validates the manifold configuration.
func (c ManifoldConfig) Validate() error {
	if c.AgentName == "" {
		return errors.NotValidf("empty AgentName")
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
	if c.HTTPClientName == "" {
		return errors.NotValidf("empty HTTPClientName")
	}
	if c.NewAPIOpen == nil {
		return errors.NotValidf("nil NewAPIOpen")
	}
	if c.NewBackendFunc == nil {
		return errors.NotValidf("nil NewBackendFunc")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the logrouter worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.HTTPClientName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, internalerrors.Capture(err)
			}

			var a agent.Agent
			if err := getter.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			var httpClientGetter corehttp.HTTPClientGetter
			if err := getter.Get(config.HTTPClientName, &httpClientGetter); err != nil {
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
				NewBackend:         config.NewBackendFunc(a, config.NewAPIOpen, httpClientGetter, config.Clock),
			})
		},
	}
}

// NewBackend returns the default backend constructor.
func NewBackend(
	agent agent.Agent,
	open func(context.Context, *api.Info, api.DialOpts) (api.Connection, error),
	httpClientGetter corehttp.HTTPClientGetter,
	clock clock.Clock,
) BackendFunc {
	return func(ctx context.Context, backendType BackendType, snapshot ConfigSnapshot) (Backend, error) {
		switch backendType {
		case BackendTypeLogSink:
			logSenderAPI := apilogsender.NewAPI(newAgentAPICaller(agent, open))
			return backends.NewLogSink(logSenderAPI, defaultBackendBufferSize)

		case BackendTypeLoki:
			httpClient, err := httpClientGetter.GetHTTPClient(ctx, corehttp.LokiPurpose)
			if err != nil {
				return nil, internalerrors.Capture(err)
			}

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

type agentAPICaller struct {
	agent  agent.Agent
	open   func(context.Context, *api.Info, api.DialOpts) (api.Connection, error)
	caller base.APICaller
}

func newAgentAPICaller(
	agent agent.Agent,
	open func(context.Context, *api.Info, api.DialOpts) (api.Connection, error),
) base.APICaller {
	return &agentAPICaller{
		agent: agent,
		open:  open,
	}
}

func (c *agentAPICaller) APICall(ctx context.Context, objType string, version int, id, request string, params, response any) error {
	caller, err := c.apiCaller(ctx)
	if err != nil {
		return err
	}
	return caller.APICall(ctx, objType, version, id, request, params, response)
}

func (c *agentAPICaller) BestFacadeVersion(facade string) int {
	if c.caller == nil {
		return 0
	}
	return c.caller.BestFacadeVersion(facade)
}

func (c *agentAPICaller) ModelTag() (names.ModelTag, bool) {
	if c.caller == nil {
		return names.ModelTag{}, false
	}
	return c.caller.ModelTag()
}

func (c *agentAPICaller) HTTPClient(scope base.HTTPClientScope) (*httprequest.Client, error) {
	caller, err := c.apiCaller(context.Background())
	if err != nil {
		return nil, err
	}
	return caller.HTTPClient(scope)
}

func (c *agentAPICaller) SimpleHTTPClient() (base.SimpleHTTPClient, error) {
	caller, err := c.apiCaller(context.Background())
	if err != nil {
		return nil, err
	}
	return caller.SimpleHTTPClient()
}

func (c *agentAPICaller) BakeryClient() base.MacaroonDischarger {
	if c.caller == nil {
		return nil
	}
	return c.caller.BakeryClient()
}

func (c *agentAPICaller) ConnectStream(ctx context.Context, path string, attrs url.Values) (base.Stream, error) {
	caller, err := c.apiCaller(ctx)
	if err != nil {
		return nil, err
	}
	return caller.ConnectStream(ctx, path, attrs)
}

func (c *agentAPICaller) ConnectControllerStream(
	ctx context.Context,
	path string,
	attrs url.Values,
	headers http.Header,
) (base.Stream, error) {
	caller, err := c.apiCaller(ctx)
	if err != nil {
		return nil, err
	}
	return caller.ConnectControllerStream(ctx, path, attrs, headers)
}

func (c *agentAPICaller) apiCaller(ctx context.Context) (base.APICaller, error) {
	if c.caller != nil {
		return c.caller, nil
	}
	info, ok := c.agent.CurrentConfig().APIInfo()
	if !ok {
		return nil, errors.New("api info not available")
	}
	conn, err := c.open(ctx, info, api.DefaultDialOpts())
	if err != nil {
		return nil, err
	}
	c.caller = conn
	return c.caller, nil
}
