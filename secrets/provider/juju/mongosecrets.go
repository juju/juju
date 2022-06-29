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
func (s secretsService) CreateSecret(ctx context.Context, URL *coresecrets.URL, p secrets.CreateParams) (*coresecrets.SecretMetadata, error) {
	metadata, err := s.backend.CreateSecret(URL, state.CreateSecretParams{
		ProviderLabel:  Provider,
		Version:        p.Version,
		Type:           p.Type,
		Owner:          p.Owner,
		Path:           p.Path,
		RotateInterval: p.RotateInterval,
		Description:    p.Description,
		Status:         p.Status,
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
func (s secretsService) GetSecretValue(ctx context.Context, URL *coresecrets.URL) (coresecrets.SecretValue, error) {
	// If no specific revision asked for, use the latest.
	if URL.Revision == 0 {
		metadata, err := s.GetSecret(ctx, URL)
		if err != nil {
			return nil, errors.Trace(err)
		}
		URL = metadata.URL.WithRevision(metadata.Revision)
	}
	return s.backend.GetSecretValue(URL)
}

// GetSecret implements SecretsService.
func (s secretsService) GetSecret(ctx context.Context, URL *coresecrets.URL) (*coresecrets.SecretMetadata, error) {
	return s.backend.GetSecret(URL.WithRevision(0).WithAttribute(""))
}

// ListSecrets implements SecretsService.
func (s secretsService) ListSecrets(ctx context.Context, filter secrets.Filter) ([]*coresecrets.SecretMetadata, error) {
	return s.backend.ListSecrets(state.SecretsFilter{})
}

// UpdateSecret implements SecretsService.
func (s secretsService) UpdateSecret(ctx context.Context, URL *coresecrets.URL, p secrets.UpdateParams) (*coresecrets.SecretMetadata, error) {
	metadata, err := s.backend.UpdateSecret(URL, state.UpdateSecretParams{
		RotateInterval: p.RotateInterval,
		Description:    p.Description,
		Status:         p.Status,
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
func (s secretsService) DeleteSecret(ctx context.Context, URL *coresecrets.URL) error {
	return errors.NotImplementedf("DeleteSecret")
}
