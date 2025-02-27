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
)

type jwtParserWorker struct {
	tomb      tomb.Tomb
	jwtParser *JWTParser
}

// Getter defines an interface to retrieve a JWTParser.
// The JWTParser is never expected to be nil but if the
// parser is not configured, the returned bool will be false.
type Getter interface {
	Get() (*JWTParser, bool)
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
func NewWorker(configGetter ControllerConfig, httpClient jwk.HTTPClient) (worker.Worker, error) {
	controllerConfig, err := configGetter.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "cannot fetch the controller config")
	}
	jwtRefreshURL := controllerConfig.LoginTokenRefreshURL()

	jwtParser := NewParserWithHTTPClient(httpClient)
	if jwtRefreshURL != "" {
		if err := jwtParser.RegisterJWKSCache(context.Background(), jwtRefreshURL); err != nil {
			return nil, err
		}
	}

	w := &jwtParserWorker{jwtParser: jwtParser}
	w.tomb.Go(w.loop)
	return w, nil
}

// Get implements Getter.
// Returns the jwt parser and a boolean
// that is false when no refresh URL is set
// i.e. when the parser is not yet configured.
func (w *jwtParserWorker) Get() (*JWTParser, bool) {
	return w.jwtParser, w.jwtParser.refreshURL != ""
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
