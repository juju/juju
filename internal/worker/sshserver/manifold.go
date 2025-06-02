// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"github.com/prometheus/client_golang/prometheus"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/api/base"
	sshserverapi "github.com/juju/juju/api/controller/sshserver"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/internal/jwtparser"
	"github.com/juju/juju/internal/sshtunneler"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/rpc/params"
)

const (
	connectionTimeout = 60 * time.Second
)

// Logger holds the methods required to log messages.
type Logger interface {
	Errorf(string, ...interface{})
	Debugf(string, ...interface{})
}

// FacadeClient represents the SSH server's facade client.
type FacadeClient interface {
	ControllerConfig() (controller.Config, error)
	WatchControllerConfig() (watcher.NotifyWatcher, error)
	SSHServerHostKey() (string, error)
	VirtualHostKey(arg params.SSHVirtualHostKeyRequestArg) ([]byte, error)
	ListPublicKeysForModel(sshPKIAuthArgs params.ListAuthorizedKeysArgs) ([]gossh.PublicKey, error)
	ResolveK8sExecInfo(arg params.SSHK8sExecArg) (params.SSHK8sExecResult, error)
	CheckSSHAccess(user string, destination virtualhostname.Info) (bool, error)
	ValidateVirtualHostname(arg virtualhostname.Info) error
}

// ManifoldConfig holds the information necessary to run an embedded SSH server
// worker in a dependency.Engine.
type ManifoldConfig struct {
	// APICallerName holds the api caller dependency name.
	APICallerName string

	// NewServerWrapperWorker is the function that creates the embedded SSH server worker.
	NewServerWrapperWorker func(ServerWrapperWorkerConfig) (worker.Worker, error)

	// NewServerWorker is the function that creates a worker that has a catacomb
	// to run the server and other worker dependencies.
	NewServerWorker func(ServerWorkerConfig) (worker.Worker, error)

	// Logger is the logger to use for the worker.
	Logger Logger

	// JWTParserName is the name of the JWT parser worker.
	JWTParserName string

	// SSHTunnelerName holds the name of the SSH tunneler worker.
	SSHTunnelerName string

	// PrometheusRegisterer is the prometheus registerer to use for metrics.
	PrometheusRegisterer prometheus.Registerer
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.NewServerWrapperWorker == nil {
		return errors.NotValidf("nil NewServerWrapperWorker")
	}
	if config.NewServerWorker == nil {
		return errors.NotValidf("nil NewServerWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.JWTParserName == "" {
		return errors.NotValidf("empty JWTParserName")
	}
	if config.SSHTunnelerName == "" {
		return errors.NotValidf("empty SSHTunnelerName")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an embedded SSH server
// worker. The manifold has no outputs.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.JWTParserName,
			config.SSHTunnelerName,
		},
		Start: config.startWrapperWorker,
	}
}

// startWrapperWorker starts the SSH server worker wrapper passing the necessary dependencies.
func (config ManifoldConfig) startWrapperWorker(context dependency.Context) (worker.Worker, error) {
	// ssh jump server is not enabled by default, but it must be enabled
	// via a feature flag.
	if !featureflag.Enabled(feature.SSHJump) {
		config.Logger.Debugf("SSH jump server worker is not enabled.")
		return nil, dependency.ErrUninstall
	}

	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	client, err := sshserverapi.NewClient(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var jwtParser *jwtparser.Parser
	if err := context.Get(config.JWTParserName, &jwtParser); err != nil {
		return nil, errors.Trace(err)
	}

	var tunnelTracker *sshtunneler.Tracker
	if err := context.Get(config.SSHTunnelerName, &tunnelTracker); err != nil {
		return nil, errors.Trace(err)
	}

	// Register the metrics collector against the prometheus register.
	metricsCollector := NewMetricsCollector()
	if err := config.PrometheusRegisterer.Register(metricsCollector); err != nil {
		return nil, errors.Trace(err)
	}

	proxyFactory := proxyFactory{
		k8sResolver: client,
		logger:      config.Logger,
		connector:   tunnelAdaper{tunnelTracker: tunnelTracker},
	}

	w, err := config.NewServerWrapperWorker(ServerWrapperWorkerConfig{
		NewServerWorker:  config.NewServerWorker,
		Logger:           config.Logger,
		FacadeClient:     client,
		ProxyFactory:     proxyFactory,
		JWTParser:        jwtParser,
		TunnelTracker:    tunnelTracker,
		metricsCollector: metricsCollector,
	})
	if err != nil {
		_ = config.PrometheusRegisterer.Unregister(metricsCollector)
		return nil, errors.Trace(err)
	}

	return common.NewCleanupWorker(w, func() {
		_ = config.PrometheusRegisterer.Unregister(metricsCollector)
	}), nil
}

// NewSSHServerListener returns a listener based on the given listener.
func NewSSHServerListener(l net.Listener, t time.Duration) net.Listener {
	return l
}

type tunnelAdaper struct {
	tunnelTracker *sshtunneler.Tracker
}

// Connect establishes a connection to the destination using the SSH tunneler.
func (t tunnelAdaper) Connect(destination virtualhostname.Info) (*gossh.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()

	machineID, _ := destination.Machine()
	req := sshtunneler.RequestArgs{
		MachineID: strconv.Itoa(machineID),
		ModelUUID: destination.ModelUUID(),
	}
	return t.tunnelTracker.RequestTunnel(ctx, req)
}
