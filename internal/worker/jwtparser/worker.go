// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/state"
)

type jwtParserWorker struct {
	tomb      tomb.Tomb
	jwtParser *JWTParser
}

type Getter interface {
	Get() *JWTParser
}

func newWorker(systemState *state.State) (worker.Worker, error) {
	controllerConfig, err := systemState.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "cannot fetch the controller config")
	}
	jwtRefreshURL := controllerConfig.LoginTokenRefreshURL()

	// If the login refresh URL is not set, we will return a nil parser.
	var jwtParser *JWTParser
	if jwtRefreshURL != "" {
		jwtParser = NewParser(jwtRefreshURL)
		if err := jwtParser.RegisterJWKSCache(context.Background()); err != nil {
			return nil, err
		}
	}

	w := &jwtParserWorker{jwtParser: jwtParser}
	w.tomb.Go(w.loop)
	return w, nil
}

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
