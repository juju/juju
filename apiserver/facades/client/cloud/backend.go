// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type Backend interface {
	state.CloudAccessor

	ControllerTag() names.ControllerTag
	Model() (Model, error)
	ModelConfig() (*config.Config, error)

	CloudCredentials(user names.UserTag, cloudName string) (map[string]state.Credential, error)
	UpdateCloudCredential(names.CloudCredentialTag, cloud.Credential) error
	RemoveCloudCredential(names.CloudCredentialTag) error
	AddCloud(cloud.Cloud) error
	AllCloudCredentials(user names.UserTag) ([]state.Credential, error)
	CredentialModelsAndOwnerAccess(tag names.CloudCredentialTag) ([]state.CredentialOwnerModelAccess, error)
}

type stateShim struct {
	*state.State
}

func NewStateBackend(st *state.State) Backend {
	return stateShim{st}
}

func (s stateShim) ModelConfig() (*config.Config, error) {
	model, err := s.State.Model()
	if err != nil {
		return nil, err
	}

	cfg, err := model.ModelConfig()
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func (s stateShim) Model() (Model, error) {
	m, err := s.State.Model()
	if err != nil {
		return nil, err
	}
	return m, nil
}

type Model interface {
	Cloud() string
	CloudCredential() (names.CloudCredentialTag, bool)
	CloudRegion() string
}
