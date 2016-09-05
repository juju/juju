// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"time"

	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/metricsender"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

// ModelManagerBackend defines methods provided by a state
// instance used by the model manager apiserver implementation.
// All the interface methods are defined directly on state.State
// and are reproduced here for use in tests.
type ModelManagerBackend interface {
	APIHostPortsGetter
	ToolsStorageGetter
	BlockGetter
	metricsender.MetricsSenderBackend
	state.CloudAccessor

	ModelUUID() string
	ModelsForUser(names.UserTag) ([]*state.UserModel, error)
	IsControllerAdmin(user names.UserTag) (bool, error)
	NewModel(state.ModelArgs) (Model, ModelManagerBackend, error)

	ComposeNewModelConfig(modelAttr map[string]interface{}, regionSpec *environs.RegionSpec) (map[string]interface{}, error)
	ControllerModel() (Model, error)
	ControllerConfig() (controller.Config, error)
	ForModel(tag names.ModelTag) (ModelManagerBackend, error)
	GetModel(names.ModelTag) (Model, error)
	Model() (Model, error)
	Unit(name string) (*state.Unit, error)
	ModelTag() names.ModelTag
	ModelConfig() (*config.Config, error)
	AllModels() ([]Model, error)
	AddModelUser(string, state.UserAccessSpec) (description.UserAccess, error)
	AddControllerUser(state.UserAccessSpec) (description.UserAccess, error)
	RemoveUserAccess(names.UserTag, names.Tag) error
	UserAccess(names.UserTag, names.Tag) (description.UserAccess, error)
	ControllerUUID() string
	ControllerTag() names.ControllerTag
	Export() (description.Model, error)
	SetUserAccess(subject names.UserTag, target names.Tag, access description.Access) (description.UserAccess, error)
	LastModelConnection(user names.UserTag) (time.Time, error)
	DumpAll() (map[string]interface{}, error)
	Close() error
}

// Model defines methods provided by a state.Model instance.
// All the interface methods are defined directly on state.Model
// and are reproduced here for use in tests.
type Model interface {
	Config() (*config.Config, error)
	Life() state.Life
	ModelTag() names.ModelTag
	Owner() names.UserTag
	Status() (status.StatusInfo, error)
	Cloud() string
	CloudCredential() (names.CloudCredentialTag, bool)
	CloudRegion() string
	Users() ([]description.UserAccess, error)
	Destroy() error
	DestroyIncludingHosted() error
}

var _ ModelManagerBackend = (*modelManagerStateShim)(nil)

type modelManagerStateShim struct {
	*state.State
}

// NewModelManagerBackend returns a modelManagerStateShim wrapping the passed
// state, which implements ModelManagerBackend.
func NewModelManagerBackend(st *state.State) ModelManagerBackend {
	return modelManagerStateShim{st}
}

// ControllerModel implements ModelManagerBackend.
func (st modelManagerStateShim) ControllerModel() (Model, error) {
	m, err := st.State.ControllerModel()
	if err != nil {
		return nil, err
	}
	return modelShim{m}, nil
}

// NewModel implements ModelManagerBackend.
func (st modelManagerStateShim) NewModel(args state.ModelArgs) (Model, ModelManagerBackend, error) {
	m, otherState, err := st.State.NewModel(args)
	if err != nil {
		return nil, nil, err
	}
	return modelShim{m}, modelManagerStateShim{otherState}, nil
}

// ForModel implements ModelManagerBackend.
func (st modelManagerStateShim) ForModel(tag names.ModelTag) (ModelManagerBackend, error) {
	otherState, err := st.State.ForModel(tag)
	if err != nil {
		return nil, err
	}
	return modelManagerStateShim{otherState}, nil
}

// GetModel implements ModelManagerBackend.
func (st modelManagerStateShim) GetModel(tag names.ModelTag) (Model, error) {
	m, err := st.State.GetModel(tag)
	if err != nil {
		return nil, err
	}
	return modelShim{m}, nil
}

// Model implements ModelManagerBackend.
func (st modelManagerStateShim) Model() (Model, error) {
	m, err := st.State.Model()
	if err != nil {
		return nil, err
	}
	return modelShim{m}, nil
}

// AllModels implements ModelManagerBackend.
func (st modelManagerStateShim) AllModels() ([]Model, error) {
	allStateModels, err := st.State.AllModels()
	if err != nil {
		return nil, err
	}
	all := make([]Model, len(allStateModels))
	for i, m := range allStateModels {
		all[i] = modelShim{m}
	}
	return all, nil
}

type modelShim struct {
	*state.Model
}

// Users implements ModelManagerBackend.
func (m modelShim) Users() ([]description.UserAccess, error) {
	stateUsers, err := m.Model.Users()
	if err != nil {
		return nil, err
	}
	users := make([]description.UserAccess, len(stateUsers))
	for i, user := range stateUsers {
		users[i] = user
	}
	return users, nil
}
