// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"context"

	"github.com/juju/errors"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/state"
)

const (
	// Provider is the name of the Juju secrets provider.
	Provider = "juju"

	// ParamBackend is the config key for the mongo secrets store.
	ParamBackend = "juju-backend"
)

type secretsService struct {
	backend state.SecretsStore
}

// NewSecretService creates a new Juju secrets service.
func NewSecretService(cfg secrets.ProviderConfig) (*secretsService, error) {
	backend, ok := cfg[ParamBackend].(*state.State)
	if !ok {
		return nil, errors.New("Juju secret store config missing state backend")
	}
	store := state.NewSecretsStore(backend)
	return &secretsService{backend: store}, nil
}

// CreateSecret implements SecretsService.
func (s secretsService) CreateSecret(ctx context.Context, uri *coresecrets.URI, p secrets.CreateParams) (*coresecrets.SecretMetadata, error) {
	metadata, err := s.backend.CreateSecret(uri, state.CreateSecretParams{
		ProviderLabel:  Provider,
		Version:        p.Version,
		Owner:          p.Owner,
		RotateInterval: p.RotateInterval,
		Description:    p.Description,
		Tags:           p.Tags,
		Params:         p.Params,
		Data:           p.Data,
	})
	if err != nil {
		return nil, errors.Annotate(err, "saving secret metadata")
	}
	return metadata, nil
}

// GetSecretValue implements SecretsService.
func (s secretsService) GetSecretValue(ctx context.Context, uri *coresecrets.URI, revision int) (coresecrets.SecretValue, error) {
	return s.backend.GetSecretValue(uri, revision)
}

// GetSecret implements SecretsService.
func (s secretsService) GetSecret(ctx context.Context, uri *coresecrets.URI) (*coresecrets.SecretMetadata, error) {
	return s.backend.GetSecret(uri)
}

// ListSecrets implements SecretsService.
func (s secretsService) ListSecrets(ctx context.Context, filter secrets.Filter) ([]*coresecrets.SecretMetadata, error) {
	return s.backend.ListSecrets(state.SecretsFilter{})
}

// UpdateSecret implements SecretsService.
func (s secretsService) UpdateSecret(ctx context.Context, uri *coresecrets.URI, p secrets.UpdateParams) (*coresecrets.SecretMetadata, error) {
	metadata, err := s.backend.UpdateSecret(uri, state.UpdateSecretParams{
		RotateInterval: p.RotateInterval,
		Description:    p.Description,
		Tags:           p.Tags,
		Params:         p.Params,
		Data:           p.Data,
	})
	if err != nil {
		return nil, errors.Annotate(err, "saving secret metadata")
	}
	return metadata, nil
}

// TODO(wallyworld)

// DeleteSecret implements SecretsService.
func (s secretsService) DeleteSecret(ctx context.Context, uri *coresecrets.URI) error {
	return errors.NotImplementedf("DeleteSecret")
}
