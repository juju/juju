// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
)

// EnvironConfigGetterFuncs holds implements environs.EnvironConfigGetter
// in a pluggable way.
type EnvironConfigGetterFuncs struct {
	ModelConfigFunc func() (*config.Config, error)
	CloudSpecFunc   func() (environs.CloudSpec, error)
}

// ModelConfig implements EnvironConfigGetter.
func (f EnvironConfigGetterFuncs) ModelConfig() (*config.Config, error) {
	return f.ModelConfigFunc()
}

// CloudSpec implements environs.EnvironConfigGetter.
func (f EnvironConfigGetterFuncs) CloudSpec() (environs.CloudSpec, error) {
	return f.CloudSpecFunc()
}

// ProviderCallContext returns the context with all necessary functionality,
// including call backs, to make a call to a cloud provider.
// TODO (anastasiamac 2018-04-30) make it real
func ProviderCallContext() context.ProviderCallContext {
	return &ProviderContext{}
}

// ProviderContext contains cloud provider call context for provider calls from
// within apiserver layer.
type ProviderContext struct{}

// InvalidateCredentialCallback implements ProviderCallContext.InvalidateCredentialCallback.
func (*ProviderContext) InvalidateCredentialCallback() error {
	return errors.NotImplementedf("InvalidateCredentialCallback")
}
