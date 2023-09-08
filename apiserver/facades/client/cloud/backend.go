// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	stdcontext "context"

	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/watcher"
	credentialservice "github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state"
)

// CloudService provides access to clouds.
type CloudService interface {
	ListAll(stdcontext.Context) ([]cloud.Cloud, error)
	Get(stdcontext.Context, string) (*cloud.Cloud, error)
	Save(ctx stdcontext.Context, cld cloud.Cloud) error
	Delete(ctx stdcontext.Context, name string) error
}

type Backend interface {
	ControllerTag() names.ControllerTag
	Model() (Model, error)
	ModelConfig(stdcontext.Context) (*config.Config, error)
	User(tag names.UserTag) (User, error)

	CloudCredentialUpdated(tag names.CloudCredentialTag) error
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
	CloudsForUser(user names.UserTag) ([]state.CloudInfo, error)
}

type CredentialService interface {
	CloudCredential(ctx stdcontext.Context, tag names.CloudCredentialTag) (cloud.Credential, error)
	AllCloudCredentials(ctx stdcontext.Context, user string) ([]credentialservice.CloudCredential, error)
	CloudCredentials(ctx stdcontext.Context, user, cloudName string) (map[string]cloud.Credential, error)
	UpdateCloudCredential(ctx stdcontext.Context, tag names.CloudCredentialTag, cred cloud.Credential) error
	RemoveCloudCredential(ctx stdcontext.Context, tag names.CloudCredentialTag) error
	WatchCredential(ctx stdcontext.Context, tag names.CloudCredentialTag) (watcher.NotifyWatcher, error)
}

type stateShim struct {
	*state.State
}

func NewStateBackend(st *state.State) Backend {
	return stateShim{st}
}

func (s stateShim) ModelConfig(ctx stdcontext.Context) (*config.Config, error) {
	model, err := s.State.Model()
	if err != nil {
		return nil, err
	}

	cfg, err := model.ModelConfig(ctx)
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
	CloudRegion() string
	CloudCredentialTag() (names.CloudCredentialTag, bool)
}

// ModelPoolBackend defines a pool of models.
type ModelPoolBackend interface {
	// GetModelCallContext gets everything that is needed to make cloud calls on behalf of the given model.
	GetModelCallContext(modelUUID string) (credentialcommon.PersistentBackend, context.ProviderCallContext, error)

	// SystemState allows access to an underlying controller state.
	SystemState() (*state.State, error)
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
