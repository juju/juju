// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/errors"
	"github.com/juju/featureflag"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/secrets/provider/juju"
	"github.com/juju/juju/secrets/provider/kubernetes"
	"github.com/juju/juju/state"
)

// StoreConfigGetter is a func used to get secret store config.
type StoreConfigGetter func() (*provider.StoreConfig, error)

// StoreConfig returns the config to create a secret store used by a model.
func StoreConfig(model *state.Model) (*provider.StoreConfig, error) {
	storeType := juju.Store
	ma := &modelAdaptor{
		model,
	}
	if model.Type() == state.ModelTypeCAAS && featureflag.Enabled(feature.SecretsStores) {
		storeType = kubernetes.Store
	}
	p, err := provider.Provider(storeType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p.StoreConfig(ma)
}

type modelAdaptor struct {
	*state.Model
}

// CloudCredential implements Model.
func (m *modelAdaptor) CloudCredential() (*cloud.Credential, error) {
	cred, ok, err := m.Model.CloudCredential()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !ok {
		return nil, nil
	}
	cloudCredentialValue := cloud.NewNamedCredential(cred.Name,
		cloud.AuthType(cred.AuthType),
		cred.Attributes,
		cred.Revoked,
	)
	return &cloudCredentialValue, nil
}
