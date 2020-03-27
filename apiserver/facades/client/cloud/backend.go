// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
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
	UpdateCloud(cloud.Cloud) error
	RemoveCloud(string) error
	AllCloudCredentials(user names.UserTag) ([]state.Credential, error)
	CredentialModelsAndOwnerAccess(tag names.CloudCredentialTag) ([]state.CredentialOwnerModelAccess, error)
	CredentialModels(tag names.CloudCredentialTag) (map[string]string, error)
	RemoveModelsCredential(tag names.CloudCredentialTag) error

	ControllerConfig() (controller.Config, error)
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
	UUID() string
	CloudName() string
	Cloud() (cloud.Cloud, error)
	CloudCredential() (state.Credential, bool, error)
	CloudRegion() string
}

// ModelPoolBackend defines a pool of models.
type ModelPoolBackend interface {
	// GetModelCallContext gets everything that is needed to make cloud calls on behalf of the given model.
	GetModelCallContext(modelUUID string) (credentialcommon.PersistentBackend, context.ProviderCallContext, error)

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

// GetModelCallContext implements ModelPoolBackend.GetModelCallContext.
func (s statePoolShim) GetModelCallContext(modelUUID string) (credentialcommon.PersistentBackend, context.ProviderCallContext, error) {
	modelState, err := s.StatePool.Get(modelUUID)
	if err != nil {
		return nil, nil, err
	}
	defer modelState.Release()
	return credentialcommon.NewPersistentBackend(modelState.State), context.CallContext(modelState.State), err
}

type User interface {
	DisplayName() string
}

func (s stateShim) User(tag names.UserTag) (User, error) {
	return s.State.User(tag)
}
