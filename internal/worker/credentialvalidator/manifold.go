// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	credentialservice "github.com/juju/juju/domain/credential/service"
	modelservice "github.com/juju/juju/domain/model/service"
	"github.com/juju/juju/internal/services"
)

// ManifoldConfig holds the dependencies and configuration for a
// Worker manifold.
type ManifoldConfig struct {
	APICallerName string

	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(context.Context, Config) (worker.Worker, error)
	Logger    logger.Logger
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.NewFacade == nil {
		return errors.NotValidf("nil NewFacade")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	var apiCaller base.APICaller
	if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}
	facade, err := config.NewFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w, err := config.NewWorker(ctx, Config{
		Facade: facade,
		Logger: config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName},
		Start:  config.start,
		Output: engine.FlagOutput,
		Filter: filterErrors,
	}
}

func filterErrors(err error) error {
	cause := errors.Cause(err)
	if cause == ErrValidityChanged ||
		cause == ErrModelCredentialChanged {
		return dependency.ErrBounce
	}
	return err
}

// ModelManifoldConfig holds the dependencies and configuration for a
// model-only Worker manifold backed by local domain services instead of
// an API caller.
type ModelManifoldConfig struct {
	DomainServicesName string
	ModelUUID          string

	NewWorker func(context.Context, Config) (worker.Worker, error)
	Logger    logger.Logger
}

// Validate is called by start to check for bad configuration.
func (config ModelManifoldConfig) Validate() error {
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// start is a StartFunc for a model-only Worker manifold.
func (config ModelManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	var domainServices services.DomainServices
	if err := getter.Get(config.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}
	facade := &modelCredentialFacade{
		credSvc:   domainServices.Credential(),
		modelSvc:  domainServices.Model(),
		modelUUID: model.UUID(config.ModelUUID),
	}
	w, err := config.NewWorker(ctx, Config{
		Facade: facade,
		Logger: config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// ModelManifold packages a Worker for use in a dependency.Engine. It reads
// credential status through local domain services rather than an API caller.
func ModelManifold(config ModelManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.DomainServicesName},
		Start:  config.start,
		Output: engine.FlagOutput,
		Filter: filterErrors,
	}
}

// modelCredentialFacade is a local facade adapter that satisfies the Facade
// interface by reading from domain services directly.
type modelCredentialFacade struct {
	credSvc   *credentialservice.WatchableService
	modelSvc  *modelservice.WatchableService
	modelUUID model.UUID
}

// ModelCredential is part of the Facade interface.
func (f *modelCredentialFacade) ModelCredential(ctx context.Context) (base.StoredCredential, bool, error) {
	key, valid, err := f.credSvc.GetModelCredentialStatus(ctx, f.modelUUID)
	if errors.Is(err, credentialerrors.ModelCredentialNotSet) {
		// Some clouds do not require a credential. Treat as valid with no
		// credential set, matching the API facade behaviour.
		return base.StoredCredential{Valid: true}, false, nil
	}
	if err != nil {
		return base.StoredCredential{}, false, errors.Trace(err)
	}
	return base.StoredCredential{
		CloudCredential: key.String(),
		Valid:           valid,
	}, true, nil
}

// WatchModelCredential is part of the Facade interface.
func (f *modelCredentialFacade) WatchModelCredential(ctx context.Context) (watcher.NotifyWatcher, error) {
	return f.modelSvc.WatchModelCloudCredential(ctx, f.modelUUID)
}
