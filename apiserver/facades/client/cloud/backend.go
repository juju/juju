// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

type Backend interface {
	state.CloudAccessor

	ControllerTag() names.ControllerTag
	Model() (Model, error)
	ModelConfig() (*config.Config, error)
	User(tag names.UserTag) (User, error)

	CloudCredentials(user names.UserTag, cloudName string) (map[string]state.Credential, error)
	UpdateCloudCredential(names.CloudCredentialTag, cloud.Credential) error
	RemoveCloudCredential(names.CloudCredentialTag) error
	AddCloud(cloud.Cloud, string) error
	RemoveCloud(string) error
	AllCloudCredentials(user names.UserTag) ([]state.Credential, error)
	CredentialModelsAndOwnerAccess(tag names.CloudCredentialTag) ([]state.CredentialOwnerModelAccess, error)
	CredentialModels(tag names.CloudCredentialTag) (map[string]string, error)

	ControllerInfo() (*state.ControllerInfo, error)
	GetCloudAccess(cloud string, user names.UserTag) (permission.Access, error)
	GetCloudUsers(cloud string) (map[string]permission.Access, error)
	CreateCloudAccess(cloud string, user names.UserTag, access permission.Access) error
	UpdateCloudAccess(cloud string, user names.UserTag, access permission.Access) error
	RemoveCloudAccess(cloud string, user names.UserTag) error
	CloudsForUser(user names.UserTag, all bool) ([]state.CloudInfo, error)
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

// ModelPoolBackend defines a pool of models.
type ModelPoolBackend interface {
	// Get allows to retrieve a particular mode given a model UUID.
	Get(modelUUID string) (PooledModelBackend, error)

	// SystemState allows access to an underlying controller state.
	SystemState() *state.State
}

type statePoolShim struct {
	*state.StatePool
}

// NewModelPoolBackend creates a model pool backend based on state.StatePool.
func NewModelPoolBackend(st *state.StatePool) ModelPoolBackend {
	return statePoolShim{st}
}

// Get implements ModelPoolBackend.Get.
func (s statePoolShim) Get(modelUUID string) (PooledModelBackend, error) {
	m, err := s.StatePool.Get(modelUUID)
	return NewPooledModelBackend(m), err
}

// PooledModelBackend defines a model retrieved from the model pool.
type PooledModelBackend interface {
	// Model represents the model itself.
	Model() credentialcommon.ModelBackend
	// Release returns a connection to the model back to the pool.
	Release() bool
}

type modelShim struct {
	*state.PooledState
}

// NewPooledModelBackend creates a pooled model backend based on state.PooledState.
func NewPooledModelBackend(st *state.PooledState) PooledModelBackend {
	return modelShim{st}
}

// Model implements PooledModelBackend.Model.
func (s modelShim) Model() credentialcommon.ModelBackend {
	return credentialcommon.NewModelBackend(s.PooledState.State)
}

type User interface {
	DisplayName() string
}

func (s stateShim) User(tag names.UserTag) (User, error) {
	return s.State.User(tag)
}
