// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
)

// ProviderStateInstances extracts the instance IDs from provider-state.
func ProviderStateInstances(env environs.Environ, stor storage.StorageReader) ([]instance.Id, error) {
	st, err := bootstrap.LoadState(stor)
	if err != nil {
		return nil, err
	}
	return st.StateInstances, nil
}
