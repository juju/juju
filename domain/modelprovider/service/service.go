// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	jujucloud "github.com/juju/juju/cloud"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/trace"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelprovider"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/errors"
	k8sprovider "github.com/juju/juju/internal/provider/kubernetes"
)

// State describes retrieval and persistence methods for credentials.
type State interface {
	// GetModelCloudAndCredential returns the cloud, cloud region
	// and credential for the given model.
	// The following errors are possible:
	// - [modelerrors.NotFound] when the model does not exist.
	GetModelCloudAndCredential(
		ctx context.Context,
		uuid coremodel.UUID,
	) (*jujucloud.Cloud, string, *modelprovider.CloudCredentialInfo, error)
}

// ProviderWithSecretToken is a subset of caas broker.
type ProviderWithSecretToken interface {
	GetSecretToken(ctx context.Context, name string) (string, error)
}

// Service provides the API for working with model providers.
type Service struct {
	modelUUID               coremodel.UUID
	st                      State
	logger                  logger.Logger
	providerWithSecretToken providertracker.ProviderGetter[ProviderWithSecretToken]
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	modelUUID coremodel.UUID,
	st State,
	logger logger.Logger,
	providerWithSecretToken providertracker.ProviderGetter[ProviderWithSecretToken],
) *Service {
	return &Service{
		modelUUID:               modelUUID,
		st:                      st,
		logger:                  logger,
		providerWithSecretToken: providerWithSecretToken,
	}
}

// GetCloudSpec returns the cloud spec for the model.
func (s *Service) GetCloudSpec(ctx context.Context) (cloudspec.CloudSpec, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	cld, cloudRegion, credInfo, err := s.st.GetModelCloudAndCredential(ctx, s.modelUUID)
	if errors.Is(err, modelerrors.NotFound) {
		err = coreerrors.NotFound
	}
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Capture(err)
	}

	var cloudCred *jujucloud.Credential
	if credInfo != nil {
		c := jujucloud.NewCredential(credInfo.AuthType, credInfo.Attributes)
		cloudCred = &c
	}
	return cloudspec.MakeCloudSpec(*cld, cloudRegion, cloudCred)
}

// GetCloudSpecForSSH returns a cloud spec suitable for sshing into a k8s container.
func (s *Service) GetCloudSpecForSSH(ctx context.Context) (cloudspec.CloudSpec, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.providerWithSecretToken(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return cloudspec.CloudSpec{}, errors.Errorf("getting secret token %w", coreerrors.NotSupported)
	}
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Capture(err)
	}

	token, err := provider.GetSecretToken(ctx, k8sprovider.ExecRBACResourceName)
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Capture(err)
	}

	cld, cloudRegion, credInfo, err := s.st.GetModelCloudAndCredential(ctx, s.modelUUID)
	if errors.Is(err, modelerrors.NotFound) {
		err = coreerrors.NotFound
	}
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Capture(err)
	}
	if credInfo == nil {
		return cloudspec.CloudSpec{}, errors.Errorf("missing credential").Add(coreerrors.NotFound)
	}

	cloudCred := jujucloud.NewCredential(credInfo.AuthType, credInfo.Attributes)
	cred, err := k8scloud.UpdateCredentialWithToken(cloudCred, token)
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Capture(err)
	}
	return cloudspec.MakeCloudSpec(*cld, cloudRegion, &cred)
}
