// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3caller

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/api"
	"github.com/juju/juju/internal/s3client"
)

// ManifoldConfig defines a Manifold's dependencies.
type ManifoldConfig struct {

	// AgentName is the name of the Agent resource that supplies
	// connection information.
	AgentName string

	// APIConfigWatcherName identifies a resource that will be
	// invalidated when api configuration changes. It's not really
	// fundamental, because it's not used directly, except to create
	// Inputs; it would be perfectly reasonable to wrap a Manifold
	// to report an extra Input instead.
	APIConfigWatcherName string

	APICallerName string

	NewS3Client func(apiConn api.Connection, logger s3client.Logger) (s3client.Session, error)

	// Filter is used to specialize responses to connection errors
	// made on behalf of different kinds of agent.
	Filter dependency.FilterFunc

	// Logger is used to write logging statements for the worker.
	Logger s3client.Logger
}

// Manifold returns a manifold whose worker wraps an S3 Session.
func Manifold(config ManifoldConfig) dependency.Manifold {
	inputs := []string{config.AgentName, config.APIConfigWatcherName, config.APICallerName}
	return dependency.Manifold{
		Inputs: inputs,
		Output: outputFunc,
		Start:  config.startFunc(),
		Filter: config.Filter,
	}
}

// startFunc returns a StartFunc that creates a S3 client based on the supplied
// manifold config and wraps it in a worker.
func (config ManifoldConfig) startFunc() dependency.StartFunc {
	return func(context dependency.Context) (worker.Worker, error) {
		var apiConn api.Connection
		if err := context.Get(config.APICallerName, &apiConn); err != nil {
			return nil, err
		}

		session, err := config.NewS3Client(apiConn, config.Logger)
		if err != nil {
			return nil, err
		}
		return newS3ClientWorker(session), nil
	}
}

// outputFunc extracts a S3 client from a *s3caller.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*s3ClientWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case *s3client.Session:
		*outPointer = inWorker.session
	default:
		return errors.Errorf("out should be *s3caller.Session; got %T", out)
	}
	return nil
}
