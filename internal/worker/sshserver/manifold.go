// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/api/base"
	sshserverapi "github.com/juju/juju/api/controller/sshserver"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
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
	HostKeyForTarget(arg params.SSHHostKeyRequestArg) ([]byte, error)
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

	// NewSSHServerListener is the function that creates a listener, based on
	// an existing listener for the server worker.
	NewSSHServerListener func(net.Listener, time.Duration) net.Listener

	// Logger is the logger to use for the worker.
	Logger Logger
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
	if config.NewSSHServerListener == nil {
		return errors.NotValidf("nil NewSSHServerListener")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an embedded SSH server
// worker. The manifold has no outputs.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Start: config.startWrapperWorker,
	}
}

// startWrapperWorker starts the SSH server worker wrapper passing the necessary dependencies.
func (config ManifoldConfig) startWrapperWorker(context dependency.Context) (worker.Worker, error) {
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

	w, err := config.NewServerWrapperWorker(ServerWrapperWorkerConfig{
		NewServerWorker:      config.NewServerWorker,
		Logger:               config.Logger,
		FacadeClient:         client,
		NewSSHServerListener: config.NewSSHServerListener,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// NewSSHServerListener returns a listener based on the given listener.
func NewSSHServerListener(l net.Listener, t time.Duration) net.Listener {
	return l
}
