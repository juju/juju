// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
)

const CloudLocalStorageDesc = "cloud local storage"

// GetCustomImageSource returns a simplestreams datasource for image metadata.
func GetCustomImageSource(env environs.Environ) (simplestreams.DataSource, error) {
	s, ok := env.(environs.EnvironStorage)
	if !ok {
		return nil, errors.NotSupportedf("provider storage")
	}
	return storage.NewStorageSimpleStreamsDataSource(CloudLocalStorageDesc, s.Storage(), storage.BaseImagesPath), nil
}
