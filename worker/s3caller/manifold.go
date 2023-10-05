// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3caller

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/api"
)

// Logger represents the logging methods called.
type Logger interface {
	Warningf(message string, args ...any)
	Debugf(message string, args ...any)
	Tracef(message string, args ...any)
}

// ManifoldConfig defines a Manifold's dependencies.
type ManifoldConfig struct {
	// APICallerName is the name of the APICaller resource that
	// supplies the API connection.
	APICallerName string

	// NewClient is used to create a new object store client.
	NewS3Client func(apiConn api.Connection, logger Logger) (Session, error)

	// Logger is used to write logging statements for the worker.
	Logger Logger
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.APICallerName == "" {
		return errors.NotValidf("nil APICallerName")
	}
	if cfg.NewS3Client == nil {
		return errors.NotValidf("nil NewS3Client")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a manifold whose worker wraps an S3 Session.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Output: outputFunc,
		Start:  config.startFunc(),
	}
}

// startFunc returns a StartFunc that creates a S3 client based on the supplied
// manifold config and wraps it in a worker.
func (config ManifoldConfig) startFunc() dependency.StartFunc {
	return func(context dependency.Context) (worker.Worker, error) {
		if err := config.Validate(); err != nil {
			return nil, errors.Trace(err)
		}

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
func outputFunc(in worker.Worker, out any) error {
	inWorker, _ := in.(*s3ClientWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case *Session:
		*outPointer = inWorker.session
	default:
		return errors.Errorf("out should be *s3caller.Session; got %T", out)
	}
	return nil
}
