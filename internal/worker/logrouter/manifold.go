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
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/loki"
	"github.com/juju/juju/internal/worker/logrouter/backends"
	"github.com/juju/juju/internal/worker/logsender"
)

// ManifoldConfig defines the names of the manifolds used by logrouter.
type ManifoldConfig struct {
	AgentName          string
	LogSource          logsender.LogRecordCh
	AgentConfigChanged *voyeur.Value
	Logger             corelogger.Logger
	Clock              clock.Clock
	HTTPClient         loki.HTTPClient
	DrainOnly          bool

	APIOpen func(context.Context, *api.Info, api.DialOpts) (api.Connection, error)
}

// Manifold returns a dependency manifold that runs the logrouter worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.AgentName},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var a agent.Agent
			if err := getter.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			logSenderAPI := apilogsender.NewAPI(newAgentAPICaller(a, config.APIOpen))
			return NewWorker(WorkerConfig{
				Agent:              a,
				LogSource:          config.LogSource,
				AgentConfigChanged: config.AgentConfigChanged,
				Logger:             config.Logger,
				Clock:              config.Clock,
				DrainOnly:          config.DrainOnly,
				ConvergeTimeout:    defaultConvergeTimeout,
				NewLogSinkBackend: func(ConfigSnapshot) (Backend, error) {
					return backends.NewLogSink(logSenderAPI, defaultBackendBufferSize)
				},
				NewLokiBackend: func(snapshot ConfigSnapshot) (Backend, error) {
					lokiConfig := loki.DefaultConfig()
					lokiConfig.HTTPClient = config.HTTPClient
					lokiConfig.Clock = config.Clock
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
				},
				NewDrainBackend: func(ConfigSnapshot) (Backend, error) {
					return backends.NewDrain(defaultBackendBufferSize)
				},
			})
		},
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
