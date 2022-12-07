// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
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
