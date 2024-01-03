// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/state"
)

// ModelAccessor exposes Model methods needed by credential validator.
type ModelAccessor interface {
	ModelTag() names.ModelTag
	CloudName() string
	WatchModelCredential() state.NotifyWatcher
}

// StateAccessor exposes State methods needed by credential validator.
type StateAccessor interface {
	credentialcommon.StateBackend
	Model() (ModelAccessor, error)
}

type stateShim struct {
	*state.State
}

// Model returns model from this shim.
func (s *stateShim) Model() (ModelAccessor, error) {
	return s.State.Model()
}

func (s *stateShim) CloudCredentialTag() (names.CloudCredentialTag, bool, error) {
	m, err := s.State.Model()
	if err != nil {
		return names.CloudCredentialTag{}, false, errors.Trace(err)
	}
	credTag, exists := m.CloudCredentialTag()
	return credTag, exists, nil
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

	// Valid indicates that this model's cloud authentication is valid.
	//
	// If this model has a cloud credential setup,
	// then this property indicates that this credential itself is valid.
	//
	// If this model has no cloud credential, then this property indicates
	// whether or not it is valid for this model to have no credential.
	// There are some clouds that do not require auth and, hence,
	// models on these clouds do not require credentials.
	//
	// If a model is on the cloud that does require credential and
	// the model's credential is not set, this property will be set to 'false'.
	Valid bool
}
