// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"

	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/sync"
	envtools "github.com/juju/juju/environs/tools"
	coretools "github.com/juju/juju/tools"
)

func UploadTestTools(
	ss envtools.SimplestreamsFetcher, store storage.Storage, stream string,
	toolsVersion version.Binary,
) (t *coretools.Tools, err error) {
	baseToolsDir, err := os.MkdirTemp("", "juju-tools")
	if err != nil {
		return nil, err
	}

	// If we exit with an error, clean up the built tools directory.
	defer func() {
		if err != nil {
			os.RemoveAll(baseToolsDir)
		}
	}()

	err = os.MkdirAll(filepath.Join(baseToolsDir, storage.BaseToolsPath, stream), 0755)
	if err != nil {
		return nil, err
	}

	storageName := envtools.StorageName(toolsVersion, stream)
	f, err := os.Create(filepath.Join(baseToolsDir, storageName))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sum, err := envtools.BundleTestTools(f)
	if err != nil {
		return nil, err
	}

	finfo, err := f.Stat()
	if err != nil {
		return nil, err
	}

	builtTools := &sync.BuiltAgent{
		Version:     toolsVersion,
		Dir:         baseToolsDir,
		StorageName: storageName,
		Size:        finfo.Size(),
		Sha256Hash:  sum,
	}
	defer os.RemoveAll(baseToolsDir)
	return SyncBuiltTools(ss, store, stream, builtTools)
}

func BuildTestTools(stream string, ver version.Binary) (_ *sync.BuiltAgent, err error) {
	// We create the entire archive before asking the environment to
	// start uploading so that we can be sure we have archived
	// correctly.
	f, err := os.CreateTemp("", "juju-tgz")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()

	sha256Hash, err := envtools.BundleTestTools(f)
	if err != nil {
		return nil, err
	}

	fileInfo, err := f.Stat()
	if err != nil {
		return nil, errors.Errorf("cannot stat newly made agent binary archive: %v", err)
	}
	size := fileInfo.Size()

	baseToolsDir, err := os.MkdirTemp("", "juju-tools")
	if err != nil {
		return nil, err
	}

	// If we exit with an error, clean up the built tools directory.
	defer func() {
		if err != nil {
			os.RemoveAll(baseToolsDir)
		}
	}()

	err = os.MkdirAll(filepath.Join(baseToolsDir, storage.BaseToolsPath, stream), 0755)
	if err != nil {
		return nil, err
	}
	storageName := envtools.StorageName(ver, stream)
	err = utils.CopyFile(filepath.Join(baseToolsDir, storageName), f.Name())
	if err != nil {
		return nil, err
	}
	return &sync.BuiltAgent{
		Version:     ver,
		Dir:         baseToolsDir,
		StorageName: storageName,
		Size:        size,
		Sha256Hash:  sha256Hash,
	}, nil
}

// SyncBuiltTools copies to storage a tools tarball and cloned copies for each series.
func SyncBuiltTools(ss envtools.SimplestreamsFetcher, store storage.Storage, stream string, builtTools *sync.BuiltAgent) (*coretools.Tools, error) {
	if err := generateAgentMetadata(ss, builtTools, stream); err != nil {
		return nil, err
	}
	syncContext := &sync.SyncContext{
		Source:            builtTools.Dir,
		TargetToolsFinder: sync.StorageToolsFinder{store},
		TargetToolsUploader: sync.StorageToolsUploader{
			Fetcher:       ss,
			Storage:       store,
			WriteMetadata: false,
			WriteMirrors:  false,
		},
		AllVersions:   true,
		Stream:        stream,
		ChosenVersion: builtTools.Version.Number,
	}
	err := sync.SyncTools(syncContext)
	if err != nil {
		return nil, err
	}
	url, err := store.URL(builtTools.StorageName)
	if err != nil {
		return nil, err
	}
	return &coretools.Tools{
		Version: builtTools.Version,
		URL:     url,
		Size:    builtTools.Size,
		SHA256:  builtTools.Sha256Hash,
	}, nil
}

// generateAgentMetadata copies the built tools tarball into a tarball for the specified
// stream and series and generates corresponding metadata.
func generateAgentMetadata(ss envtools.SimplestreamsFetcher, toolsInfo *sync.BuiltAgent, stream string) error {
	// Copy the tools to the target storage, recording a Tools struct for each one.
	var targetTools coretools.List
	targetTools = append(targetTools, &coretools.Tools{
		Version: toolsInfo.Version,
		Size:    toolsInfo.Size,
		SHA256:  toolsInfo.Sha256Hash,
	})
	// The tools have been copied to a temp location from which they will be uploaded,
	// now write out the matching simplestreams metadata so that SyncTools can find them.
	metadataStore, err := filestorage.NewFileStorageWriter(toolsInfo.Dir)
	if err != nil {
		return err
	}
	return envtools.MergeAndWriteMetadata(ss, metadataStore, stream, stream, targetTools, false)
}
