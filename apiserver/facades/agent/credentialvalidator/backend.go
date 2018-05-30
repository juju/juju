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

	// ModelUsesCredential determines if the model uses given cloud credential.
	ModelUsesCredential(tag names.CloudCredentialTag) (bool, error)

	// ModelCredential retrieves the cloud credential that a model uses.
	ModelCredential() (*ModelCredential, error)

	// WatchCredential returns a watcher that is keeping an eye on all changes to
	// a given cloud credential.
	WatchCredential(names.CloudCredentialTag) state.NotifyWatcher

	// InvalidateModelCredential marks the cloud credential that a current model
	// uses as invalid.
	InvalidateModelCredential(reason string) error
}

func NewBackend(st StateAccessor) Backend {
	return &backend{st}
}

type backend struct {
	StateAccessor
}

// ModelUsesCredential implements Backend.ModelUsesCredential.
func (b *backend) ModelUsesCredential(tag names.CloudCredentialTag) (bool, error) {
	m, err := b.Model()
	if err != nil {
		return false, errors.Trace(err)
	}
	modelCredentialTag, exists := m.CloudCredential()
	return exists && tag == modelCredentialTag, nil
}

// ModelCredential implements Backend.ModelCredential.
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
