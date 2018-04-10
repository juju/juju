// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

// Backend defines behavior that credential validator needs.
type Backend interface {
	ModelUsesCredential(tag names.CloudCredentialTag) (bool, error)
	ModelCredential() (*ModelCredential, error)
	WatchCredential(names.CloudCredentialTag) state.NotifyWatcher
}

func NewBackend(st StateAccessor) Backend {
	return &backend{st}
}

type backend struct {
	StateAccessor
}

func (b *backend) ModelUsesCredential(tag names.CloudCredentialTag) (bool, error) {
	m, err := b.Model()
	if err != nil {
		return false, errors.Trace(err)
	}
	modelCredentialTag, exists := m.CloudCredential()
	return exists && tag == modelCredentialTag, nil
}

func (b *backend) ModelCredential() (*ModelCredential, error) {
	m, err := b.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelCredentialTag, exists := m.CloudCredential()
	result := &ModelCredential{Model: m.ModelTag(), Exists: exists}
	if !exists {
		return result, nil
	}

	result.Credential = modelCredentialTag
	credential, err := b.CloudCredential(modelCredentialTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.Valid = credential.IsValid()
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
