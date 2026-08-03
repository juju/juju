//go:build dqlite

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrainworker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/api/common/secretsdrain"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/secret"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	jujusecrets "github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/services"
)

// ModelManifoldConfig holds the dependencies and configuration for a
// model-only Worker manifold backed by local domain services instead of
// an API caller.
type ModelManifoldConfig struct {
	DomainServicesName string
	ModelUUID          string
	Logger             logger.Logger

	NewWorker func(Config) (worker.Worker, error)
}

// Validate is called by start to check for bad configuration.
func (cfg ModelManifoldConfig) Validate() error {
	if cfg.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if cfg.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// start is a StartFunc for a model-only Worker manifold.
func (cfg ModelManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServices services.DomainServices
	if err := getter.Get(cfg.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}

	secretSvc := domainServices.Secret()
	backendSvc := domainServices.SecretBackend()
	modelUUID := model.UUID(cfg.ModelUUID)

	facade := &modelSecretsDrainFacade{
		secretSvc:  secretSvc,
		backendSvc: backendSvc,
		modelUUID:  modelUUID,
	}

	backendGetter := func() (jujusecrets.BackendsClient, error) {
		api := &localJujuAPIClient{
			secretSvc:  secretSvc,
			backendSvc: backendSvc,
			modelUUID:  modelUUID,
		}
		return jujusecrets.NewClient(api)
	}

	w, err := cfg.NewWorker(Config{
		SecretsDrainFacade:   facade,
		Logger:               cfg.Logger,
		SecretsBackendGetter: backendGetter,
		LeadershipTrackerFunc: func() leadership.ChangeTracker {
			return passThroughLeadershipTracker{}
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// ModelManifold packages a Worker for use in a dependency.Engine. It reads
// secret drain data through local domain services rather than an API caller.
func ModelManifold(config ModelManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.DomainServicesName},
		Start:  config.start,
	}
}

// modelSecretsDrainFacade satisfies the SecretsDrainFacade interface by
// reading from domain services directly.
type modelSecretsDrainFacade struct {
	secretSvc  secretService
	backendSvc secretBackendService
	modelUUID  model.UUID
}

// secretService is the subset of the domain secret service needed by the
// model drain facade adapter.
type secretService interface {
	ListUserSecretsToDrain(context.Context) ([]*coresecrets.SecretMetadataForDrain, error)
	ChangeSecretBackend(context.Context, *coresecrets.URI, int, secretservice.ChangeSecretBackendParams) error
	GetSecretValue(context.Context, *coresecrets.URI, int, secret.SecretAccessor) (coresecrets.SecretValue, *coresecrets.ValueRef, error)
	ListGrantedSecretsForBackend(context.Context, string, coresecrets.SecretRole, ...secret.SecretAccessor) ([]*coresecrets.SecretRevisionRef, error)
}

// secretBackendService is the subset of the domain secret backend service
// needed by the model drain facade adapter.
type secretBackendService interface {
	GetRevisionsToDrain(context.Context, model.UUID, []coresecrets.SecretExternalRevision) ([]secretbackendservice.RevisionInfo, error)
	WatchModelSecretBackendChanged(context.Context, model.UUID) (watcher.NotifyWatcher, error)
	DrainBackendConfigInfo(context.Context, secretbackendservice.DrainBackendConfigParams) (*provider.ModelBackendConfigInfo, error)
	BackendConfigInfo(context.Context, secretbackendservice.BackendConfigParams) (*provider.ModelBackendConfigInfo, error)
}

// GetSecretsToDrain is part of the SecretsDrainFacade interface.
func (f *modelSecretsDrainFacade) GetSecretsToDrain(ctx context.Context) ([]coresecrets.SecretMetadataForDrain, error) {
	secrets, err := f.secretSvc.ListUserSecretsToDrain(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []coresecrets.SecretMetadataForDrain
	for _, info := range secrets {
		revisions, err := f.backendSvc.GetRevisionsToDrain(ctx, f.modelUUID, info.Revisions)
		if err != nil {
			return nil, errors.Trace(err)
		}
		var extRevs []coresecrets.SecretExternalRevision
		for _, r := range revisions {
			extRevs = append(extRevs, coresecrets.SecretExternalRevision{
				Revision: r.Revision,
				ValueRef: r.ValueRef,
			})
		}
		if len(extRevs) == 0 {
			continue
		}
		result = append(result, coresecrets.SecretMetadataForDrain{
			URI:       info.URI,
			Revisions: extRevs,
		})
	}
	return result, nil
}

// ChangeSecretBackend is part of the SecretsDrainFacade interface.
func (f *modelSecretsDrainFacade) ChangeSecretBackend(ctx context.Context, args []secretsdrain.ChangeSecretBackendArg) (secretsdrain.ChangeSecretBackendResult, error) {
	result := secretsdrain.ChangeSecretBackendResult{
		Results: make([]error, len(args)),
	}
	accessor := secret.SecretAccessor{
		Kind: secret.ModelAccessor,
		ID:   f.modelUUID.String(),
	}
	for i, arg := range args {
		err := f.secretSvc.ChangeSecretBackend(ctx, arg.URI, arg.Revision, secretservice.ChangeSecretBackendParams{
			Accessor: accessor,
			ValueRef: arg.ValueRef,
			Data:     arg.Data,
		})
		result.Results[i] = err
	}
	return result, nil
}

// WatchSecretBackendChanged is part of the SecretsDrainFacade interface.
func (f *modelSecretsDrainFacade) WatchSecretBackendChanged(ctx context.Context) (watcher.NotifyWatcher, error) {
	return f.backendSvc.WatchModelSecretBackendChanged(ctx, f.modelUUID)
}

// localJujuAPIClient is a local implementation of the jujusecrets.JujuAPIClient
// interface that reads from domain services directly instead of making API calls.
type localJujuAPIClient struct {
	secretSvc  secretService
	backendSvc secretBackendService
	modelUUID  model.UUID
}

// GetBackendConfigForDrain returns the backend config for the drain worker.
func (c *localJujuAPIClient) GetBackendConfigForDrain(ctx context.Context, backendID *string) (*provider.ModelBackendConfig, string, error) {
	var bid string
	if backendID != nil {
		bid = *backendID
	}
	accessor := secret.SecretAccessor{
		Kind: secret.ModelAccessor,
		ID:   c.modelUUID.String(),
	}
	info, err := c.backendSvc.DrainBackendConfigInfo(ctx, secretbackendservice.DrainBackendConfigParams{
		GrantedSecretsGetter: c.secretSvc.ListGrantedSecretsForBackend,
		Accessor:             accessor,
		ModelUUID:            c.modelUUID,
		BackendID:            bid,
	})
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	want := info.ActiveID
	if backendID != nil {
		want = *backendID
	}
	cfg, ok := info.Configs[want]
	if !ok {
		return nil, "", errors.Errorf("secret backend %q missing from config", want)
	}
	return &cfg, info.ActiveID, nil
}

// GetRevisionContentInfo returns the content of a secret revision.
func (c *localJujuAPIClient) GetRevisionContentInfo(ctx context.Context, uri *coresecrets.URI, revision int, _ bool) (*jujusecrets.ContentParams, *provider.ModelBackendConfig, bool, error) {
	accessor := secret.SecretAccessor{
		Kind: secret.ModelAccessor,
		ID:   c.modelUUID.String(),
	}
	val, valueRef, err := c.secretSvc.GetSecretValue(ctx, uri, revision, accessor)
	if err != nil {
		return nil, nil, false, errors.Trace(err)
	}
	content := &jujusecrets.ContentParams{SecretValue: val, ValueRef: valueRef}
	if content.ValueRef == nil {
		return content, nil, false, nil
	}
	backendID := content.ValueRef.BackendID
	cfg, _, err := c.GetBackendConfigForDrain(ctx, &backendID)
	if err != nil {
		return nil, nil, false, errors.Trace(err)
	}
	return content, cfg, false, nil
}

// GetSecretBackendConfig returns the backend config for reading secrets.
func (c *localJujuAPIClient) GetSecretBackendConfig(ctx context.Context, backendID *string) (*provider.ModelBackendConfigInfo, error) {
	var bids []string
	if backendID != nil {
		bids = []string{*backendID}
	}
	accessor := secret.SecretAccessor{
		Kind: secret.ModelAccessor,
		ID:   c.modelUUID.String(),
	}
	return c.backendSvc.BackendConfigInfo(ctx, secretbackendservice.BackendConfigParams{
		GrantedSecretsGetter: c.secretSvc.ListGrantedSecretsForBackend,
		Accessor:             accessor,
		ModelUUID:            c.modelUUID,
		BackendIDs:           bids,
		SameController:       true,
	})
}

// GetContentInfo returns info about the content of a secret.
func (c *localJujuAPIClient) GetContentInfo(ctx context.Context, uri *coresecrets.URI, _ string, _, _ bool) (*jujusecrets.ContentParams, *provider.ModelBackendConfig, bool, error) {
	return c.GetRevisionContentInfo(ctx, uri, 0, false)
}
