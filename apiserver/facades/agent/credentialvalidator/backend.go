// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	jujucloud "github.com/juju/juju/cloud"
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

	// WatchModelCredential returns a watcher that is keeping an eye on what cloud credential a model uses.
	WatchModelCredential() (state.NotifyWatcher, error)
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
	modelCredentialTag, exists := m.CloudCredentialTag()
	return exists && tag == modelCredentialTag, nil
}

// ModelCredential implements Backend.ModelCredential.
func (b *backend) ModelCredential() (*ModelCredential, error) {
	m, err := b.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelCredentialTag, exists := m.CloudCredentialTag()
	result := &ModelCredential{Model: m.ModelTag(), Exists: exists}
	if !exists {
		// A model credential is not set, we must check if the model
		// is on the cloud that requires a credential.
		supportsEmptyAuth, err := b.cloudSupportsNoAuth(m.CloudName())
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Valid = supportsEmptyAuth
		if !supportsEmptyAuth {
			// TODO (anastasiamac 2018-11-12) Figure out how to notify the users here - maybe set a model status?...
			logger.Warningf("model credential is not set for the model but the cloud requires it")
		}
		return result, nil
	}

	result.Credential = modelCredentialTag
	credential, err := b.CloudCredential(modelCredentialTag)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		// In this situation, a model refers to a credential that does not exist in credentials collection.
		// TODO (anastasiamac 2018-11-12) Figure out how to notify the users here - maybe set a model status?...
		logger.Warningf("cloud credential reference is set for the model but the credential content is no longer on the controller")
		result.Valid = false
		return result, nil
	}
	result.Valid = credential.IsValid()
	return result, nil
}

// WatchModelCredential implements Backend.WatchModelCredential.
func (b *backend) WatchModelCredential() (state.NotifyWatcher, error) {
	m, err := b.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return m.WatchModelCredential(), nil
}

func (b *backend) cloudSupportsNoAuth(cloudName string) (bool, error) {
	cloud, err := b.Cloud(cloudName)
	if err != nil {
		return false, errors.Trace(err)
	}
	for _, authType := range cloud.AuthTypes {
		if authType == jujucloud.EmptyAuthType {
			return true, nil
		}
	}
	return false, nil
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
