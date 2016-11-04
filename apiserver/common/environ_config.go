// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

// EnvironConfigGetterFuncs holds implements environs.EnvironConfigGetter
// in a pluggable way.
type EnvironConfigGetterFuncs struct {
	ModelConfigFunc func() (*config.Config, error)
	CloudSpecFunc   func(names.ModelTag) (environs.CloudSpec, error)
}

// ModelConfig implements EnvironConfigGetter.
func (f EnvironConfigGetterFuncs) ModelConfig() (*config.Config, error) {
	return f.ModelConfigFunc()
}

// CloudSpec implements environs.EnvironConfigGetter.
func (f EnvironConfigGetterFuncs) CloudSpec(model names.ModelTag) (environs.CloudSpec, error) {
	return f.CloudSpecFunc(model)
}
