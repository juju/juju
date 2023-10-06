// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// Model defines a subset of state model methods.
type Model interface {
	ControllerUUID() string
	Cloud() (cloud.Cloud, error)
	CloudCredential() (Credential, error)
	Config() (*config.Config, error)
	UUID() string
	Name() string
	Type() state.ModelType
	State() *state.State
}

type StatePool interface {
	GetModel(modelUUID string) (common.Model, func() bool, error)
}

type SecretsBackendState interface {
	GetSecretBackendByID(ID string) (*secrets.SecretBackend, error)
	ListSecretBackends() ([]*secrets.SecretBackend, error)
}

// SecretsConsumer instances provide secret consumer apis.
type SecretsConsumer interface {
	SecretAccess(uri *secrets.URI, subject names.Tag) (secrets.SecretRole, error)
}

// SecretsState instances provide secret apis.
type SecretsState interface {
	ListModelSecrets(all bool) (map[string]set.Strings, error)
}

// SecretsMetaState instances provide secret metadata apis.
type SecretsMetaState interface {
	ListSecrets(state.SecretsFilter) ([]*secrets.SecretMetadata, error)
	ListSecretRevisions(uri *secrets.URI) ([]*secrets.SecretRevisionMetadata, error)
}

// SecretsRemoveState instances provide secret removal apis.
type SecretsRemoveState interface {
	DeleteSecret(*secrets.URI, ...int) ([]secrets.ValueRef, error)
	GetSecret(*secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretRevision(uri *secrets.URI, revision int) (*secrets.SecretRevisionMetadata, error)
}

// Credential represents a cloud credential.
type Credential interface {
	AuthType() string
	Attributes() map[string]string
}

// SecretsModel wraps a state Model.
func SecretsModel(m *state.Model) Model {
	return &modelShim{m}
}

type modelShim struct {
	*state.Model
}

func (m *modelShim) CloudCredential() (Credential, error) {
	cred, ok, err := m.Model.CloudCredential()
	if !ok {
		return nil, errors.New("missing model credential")
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &credentialShim{cred}, nil
}

type credentialShim struct {
	state.Credential
}

func (c *credentialShim) AuthType() string {
	return c.Credential.AuthType
}

func (c *credentialShim) Attributes() map[string]string {
	return c.Credential.Attributes
}

type SecretsGetter interface {
	GetSecret(*secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretValue(*secrets.URI, int) (secrets.SecretValue, *secrets.ValueRef, error)
}
