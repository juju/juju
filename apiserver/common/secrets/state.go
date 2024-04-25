// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// Model defines a subset of state model methods.
type Model interface {
	ControllerUUID() string
	CloudName() string
	CloudCredentialTag() (names.CloudCredentialTag, bool)
	Config() (*config.Config, error)
	UUID() string
	Name() string
	Type() state.ModelType
	State() *state.State

	ModelConfig(context.Context) (*config.Config, error)
	WatchForModelConfigChanges() state.NotifyWatcher
}

type StatePool interface {
	GetModel(modelUUID string) (common.Model, func() bool, error)
}

// SecretsModel wraps a state Model.
func SecretsModel(m *state.Model) Model {
	return &modelShim{m}
}

type modelShim struct {
	*state.Model
}
