// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/jwtparser"
)

type jwtParserWorker struct {
	tomb      tomb.Tomb
	jwtParser *jwtparser.Parser
}

// ControllerConfig defines an interface to retrieve controller config.
type ControllerConfig interface {
	ControllerConfig() (controller.Config, error)
}

// HTTPClient defines an interface for an HTTP client
// with a single GET method.
type HTTPClient interface {
	jwk.HTTPClient
}

// NewWorker returns a worker that provides a JWTParser.
func NewWorker(configGetter ControllerConfig, httpClient HTTPClient) (worker.Worker, error) {
	controllerConfig, err := configGetter.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "cannot fetch the controller config")
	}
	jwtRefreshURL := controllerConfig.LoginTokenRefreshURL()

	ctx, done := context.WithCancel(context.Background())
	jwtParser := jwtparser.NewParserWithHTTPClient(ctx, httpClient)
	if jwtRefreshURL != "" {
		if err := jwtParser.SetJWKSCache(context.Background(), jwtRefreshURL); err != nil {
			done()
			return nil, errors.Annotate(err, "cannot register JWKS cache")
		}
	}

	w := &jwtParserWorker{jwtParser: jwtParser}
	w.tomb.Go(func() error {
		defer done()
		return w.loop()
	})
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *jwtParserWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *jwtParserWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *jwtParserWorker) loop() (err error) {
	<-w.tomb.Dying()
	return tomb.ErrDying
}
