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
// If a JWTParser is not available, it will return nil.
type Getter interface {
	Get() *JWTParser
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

	// If the login refresh URL is not set, we will return a nil parser.
	var jwtParser *JWTParser
	if jwtRefreshURL != "" {
		jwtParser = NewParserWithHTTPClient(httpClient, jwtRefreshURL)
		if err := jwtParser.RegisterJWKSCache(context.Background()); err != nil {
			return nil, err
		}
	}

	w := &jwtParserWorker{jwtParser: jwtParser}
	w.tomb.Go(w.loop)
	return w, nil
}

// Get implements Getter.
func (w *jwtParserWorker) Get() *JWTParser {
	return w.jwtParser
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
