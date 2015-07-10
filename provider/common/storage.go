// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
)

// GetCustomImageSource returns a data source for the cloud local storage.
func GetCustomImageSource(env environs.Environ) (simplestreams.DataSource, error) {
	s, ok := env.(storage.Storage)
	if !ok {
		return nil, errors.NotSupportedf("provider does not support storage")
	}
	return storage.NewStorageSimpleStreamsDataSource("cloud local storage", s, storage.BaseImagesPath), nil
}
