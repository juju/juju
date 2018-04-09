// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

// Backend exposes information about any current model migrations.
type Backend interface {
	ModelUUID() string
	ModelCredential() (*ModelCredential, error)
	ModelUsesCredential(tag names.CloudCredentialTag) (bool, error)
	WatchCredential(names.CloudCredentialTag) state.NotifyWatcher
}

type stateShim struct {
	*state.State
}

// NewStateBackend creates new backend to be used by credential validator Facade.
func NewStateBackend(st *state.State) Backend {
	return &stateShim{st}
}

// ModelUsesCredential determines if the model for this backend
// uses given credential.
func (s *stateShim) ModelUsesCredential(tag names.CloudCredentialTag) (bool, error) {
	m, err := s.Model()
	if err != nil {
		return false, errors.Trace(err)
	}
	currentModelName := m.Name()
	models, err := s.CredentialModelsAndOwnerAccess(tag)
	if err != nil {
		return false, errors.Trace(err)
	}

	for _, credentialModel := range models {
		if credentialModel.ModelName == currentModelName {
			return true, nil
			break
		}
	}
	return false, nil
}

// ModelCredential implements Backend interface.
func (s *stateShim) ModelCredential() (*ModelCredential, error) {
	model, err := s.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := &ModelCredential{Model: model.ModelTag()}
	tag, exists := model.CloudCredential()
	result.Exists = exists
	if !exists {
		// Model is on the cloud with "empty" auth and, hence,
		// model credential is not set.
		return result, nil
	}

	result.Credential = tag
	c, err := s.CloudCredential(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.Valid = c.IsValid()

	return result, nil
}

// ModelCredential stores model's cloud credential information.
type ModelCredential struct {
	// Model is a model tag.
	Model names.ModelTag

	// Exists indicates whether the model has  a cloud credential.
	// On some clouds, that only require "empty" auth, cloud credential
	// is not needed for the models to function properly.
	Exists bool

	// Credential is a cloud credential tag.
	Credential names.CloudCredentialTag

	// Valid indicates that Juju considers this cloud credential to be valid.
	Valid bool
}
