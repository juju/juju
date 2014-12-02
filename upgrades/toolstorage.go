// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"bytes"
	"io"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/provider"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/toolstorage"
	"github.com/juju/juju/tools"
)

var stateToolsStorage = (*state.State).ToolsStorage

// migrateToolsStorage copies tools from provider storage to
// environment storage.
func migrateToolsStorage(st *state.State, agentConfig agent.Config) error {
	logger.Debugf("migrating tools to environment storage")

	tstor, err := stateToolsStorage(st)
	if err != nil {
		return errors.Annotate(err, "cannot get tools storage")
	}
	defer tstor.Close()

	// Local and manual provider host storage on the state server's
	// filesystem, and serve via HTTP storage. The storage worker
	// doesn't run yet, so we just open the files directly.
	var stor storage.StorageReader
	providerType := agentConfig.Value(agent.ProviderType)
	if providerType == provider.Local || provider.IsManual(providerType) {
		storageDir := agentConfig.Value(agent.StorageDir)
		var err error
		stor, err = filestorage.NewFileStorageReader(storageDir)
		if err != nil {
			return errors.Annotate(err, "cannot get local filesystem storage reader")
		}
	} else {
		var err error
		stor, err = environs.LegacyStorage(st)
		if errors.IsNotSupported(err) {
			return nil
		} else if err != nil {
			return errors.Annotate(err, "cannot get provider storage")
		}
	}

	// Search provider storage for tools.
	datasource := storage.NewStorageSimpleStreamsDataSource("provider storage", stor, storage.BaseToolsPath)
	toolsList, err := envtools.FindToolsForCloud(
		[]simplestreams.DataSource{datasource},
		simplestreams.CloudSpec{},
		envtools.ReleasedStream,
		-1, -1, tools.Filter{})
	switch err {
	case nil:
		break
	case tools.ErrNoMatches, envtools.ErrNoTools:
		// No tools in provider storage: nothing to do.
		return nil
	default:
		return errors.Annotate(err, "cannot find tools in provider storage")
	}

	for _, agentTools := range toolsList {
		logger.Infof("migrating %v tools to environment storage", agentTools.Version)
		data, err := fetchToolsArchive(stor, envtools.LegacyReleaseDirectory, agentTools)
		if err != nil {
			return errors.Annotatef(err, "failed to fetch %v tools", agentTools.Version)
		}
		err = tstor.AddTools(bytes.NewReader(data), toolstorage.Metadata{
			Version: agentTools.Version,
			Size:    agentTools.Size,
			SHA256:  agentTools.SHA256,
		})
		if err != nil {
			return errors.Annotatef(err, "failed to add %v tools to environment storage", agentTools.Version)
		}
	}
	return nil
}

func fetchToolsArchive(stor storage.StorageReader, toolsDir string, agentTools *tools.Tools) ([]byte, error) {
	r, err := stor.Get(envtools.StorageName(agentTools.Version, toolsDir))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var buf bytes.Buffer
	hash, size, err := utils.ReadSHA256(io.TeeReader(r, &buf))
	if err != nil {
		return nil, err
	}
	if hash != agentTools.SHA256 {
		return nil, errors.New("hash mismatch")
	}
	if size != agentTools.Size {
		return nil, errors.New("size mismatch")
	}
	return buf.Bytes(), nil
}
