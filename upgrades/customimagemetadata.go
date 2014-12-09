// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"bytes"
	"io/ioutil"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/provider"
	"github.com/juju/juju/state"
	statestorage "github.com/juju/juju/state/storage"
)

var newStateStorage = statestorage.NewStorage

// migrateCustomImageMetadata copies uploaded image metadata from provider
// storage to environment storage, preserving paths.
func migrateCustomImageMetadata(st *state.State, agentConfig agent.Config) error {
	logger.Debugf("migrating custom image metadata to environment storage")
	estor := newStateStorage(st.EnvironUUID(), st.MongoSession())

	// Local and manual provider host storage on the state server's
	// filesystem, and serve via HTTP storage. The storage worker
	// doesn't run yet, so we just open the files directly.
	var pstor storage.StorageReader
	providerType := agentConfig.Value(agent.ProviderType)
	if providerType == provider.Local || provider.IsManual(providerType) {
		storageDir := agentConfig.Value(agent.StorageDir)
		var err error
		pstor, err = filestorage.NewFileStorageReader(storageDir)
		if err != nil {
			return errors.Annotate(err, "cannot get local filesystem storage reader")
		}
	} else {
		var err error
		pstor, err = environs.LegacyStorage(st)
		if errors.IsNotSupported(err) {
			return nil
		} else if err != nil {
			return errors.Annotate(err, "cannot get provider storage")
		}
	}

	paths, err := pstor.List(storage.BaseImagesPath)
	if err != nil {
		return err
	}
	for _, path := range paths {
		logger.Infof("migrating image metadata at path %q", path)
		data, err := readImageMetadata(pstor, path)
		if err != nil {
			return errors.Annotate(err, "failed to read image metadata")
		}
		err = estor.Put(path, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return errors.Annotate(err, "failed to write image metadata")
		}
	}
	return nil
}

func readImageMetadata(stor storage.StorageReader, path string) ([]byte, error) {
	r, err := stor.Get(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return ioutil.ReadAll(r)
}
