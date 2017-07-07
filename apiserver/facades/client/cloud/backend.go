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
	Clouds() (map[names.CloudTag]cloud.Cloud, error)
	Cloud(cloudName string) (cloud.Cloud, error)
	CloudCredentials(user names.UserTag, cloudName string) (map[string]cloud.Credential, error)
	CloudCredential(tag names.CloudCredentialTag) (cloud.Credential, error)
	ControllerModel() (Model, error)
	ControllerTag() names.ControllerTag
	ModelTag() names.ModelTag
	ModelConfig() (*config.Config, error)
	UpdateCloudCredential(names.CloudCredentialTag, cloud.Credential) error
	RemoveCloudCredential(names.CloudCredentialTag) error

	IsControllerAdmin(names.UserTag) (bool, error)

	GetModel(tag names.ModelTag) (Model, error)

	Close() error
}

type stateShim struct {
	*state.State
}

func NewStateBackend(st *state.State) Backend {
	return stateShim{st}
}

func (s stateShim) ControllerModel() (Model, error) {
	m, err := s.State.ControllerModel()
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s stateShim) GetModel(tag names.ModelTag) (Model, error) {
	m, err := s.State.GetModel(tag)
	if err != nil {
		return nil, err
	}
	return m, nil
}

type Model interface {
	Cloud() string
	CloudCredential() (names.CloudCredentialTag, bool)
	CloudRegion() string
	ModelTag() names.ModelTag

	Config() (*config.Config, error)
}
