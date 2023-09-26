// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
)

// State is the model state required by this service.
type State interface {
	// SetCloudCredential sets the cloud credential for the given mode.
	SetCloudCredential(context.Context, model.UUID, credential.ID) error
}

// Service defines a service for interacting with the underlying state based
// information of a model.
type Service struct {
	st State
}

// NewService returns a new Service for interacting with a models state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// SetCloudCredential takes a cloud credential tag to set for this model.
func (s *Service) SetCloudCredential(
	ctx context.Context,
	modelUUID model.UUID,
	cred names.CloudCredentialTag,
) error {
	id := credential.ID{
		Cloud: cred.Cloud().Id(),
		Owner: cred.Owner().Id(),
		Name:  cred.Name(),
	}
	err := s.st.SetCloudCredential(ctx, modelUUID, id)
	if errors.Is(err, errors.NotFound) {
		return fmt.Errorf("cloud credential %q %w", cred.String(), errors.NotFound)
	} else if errors.Is(err, modelerrors.NotFound) {
		return err
	} else if err != nil {
		return fmt.Errorf("setting cloud credential on model: %w", err)
	}
	return nil
}
