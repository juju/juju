// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// EnvironConfigGetterFuncs holds implements environs.EnvironConfigGetter
// in a pluggable way.
type EnvironConfigGetterFuncs struct {
	ModelConfigFunc func() (*config.Config, error)
	CloudSpecFunc   func() (environscloudspec.CloudSpec, error)
	controllerUUID  string
}

func NewEnvironConfigGetterFuncs(
	modelConfigFunc func() (*config.Config, error),
	cloudSpecFunc func() (environscloudspec.CloudSpec, error),
	controllerUUID string,
) EnvironConfigGetterFuncs {
	return EnvironConfigGetterFuncs{
		ModelConfigFunc: modelConfigFunc,
		CloudSpecFunc:   cloudSpecFunc,
		controllerUUID:  controllerUUID,
	}
}

// ModelConfig implements EnvironConfigGetter.
func (f EnvironConfigGetterFuncs) ModelConfig() (*config.Config, error) {
	return f.ModelConfigFunc()
}

// CloudSpec implements environs.EnvironConfigGetter.
func (f EnvironConfigGetterFuncs) CloudSpec() (environscloudspec.CloudSpec, error) {
	return f.CloudSpecFunc()
}

// ControllerUUID implements environs.EnvironConfigGetter.
func (f EnvironConfigGetterFuncs) ControllerUUID() string {
	return f.controllerUUID
}

// NewEnvironFunc is a function that returns a BootstrapEnviron instance.
type NewEnvironFunc func() (environs.BootstrapEnviron, error)

// EnvironFuncForModel is a helper function that returns a NewEnvironFunc suitable for
// the specified model.
func EnvironFuncForModel(model stateenvirons.Model, configGetter environs.EnvironConfigGetter) NewEnvironFunc {
	if model.Type() == state.ModelTypeCAAS {
		return func() (environs.BootstrapEnviron, error) {
			f := stateenvirons.GetNewCAASBrokerFunc(caas.New)
			return f(model)
		}
	}
	return func() (environs.BootstrapEnviron, error) {
		return environs.GetEnviron(configGetter, environs.New)
	}
}
