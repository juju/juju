// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/downloader"
	"github.com/juju/juju/internal/charm/services"
	"github.com/juju/juju/state"
)

// CharmUploader is an interface that is used to update the charm in
// state and upload it to the object store.
type CharmUploader interface {
	UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error)
	PrepareCharmUpload(curl string) (services.UploadedCharm, error)
	ModelUUID() string
}

// RepackageAndUploadCharm expands the given charm archive to a
// temporary directory, repackages it with the given curl's revision,
// then uploads it to storage, and finally updates the state.
func RepackageAndUploadCharm(
	ctx context.Context,
	objectStore services.Storage,
	uploader CharmUploader,
	archive *charm.CharmArchive,
	curl string,
	charmRevision int,
) (charm.Charm, string, string, string, error) {
	// Create a temp dir to contain the extracted charm dir.
	tempDir, err := os.MkdirTemp("", "charm-download")
	if err != nil {
		return nil, "", "", "", errors.Annotate(err, "cannot create temp directory")
	}
	defer os.RemoveAll(tempDir)
	extractPath := filepath.Join(tempDir, "extracted")

	// Expand and repack it with the specified revision
	archive.SetRevision(charmRevision)
	if err := archive.ExpandTo(extractPath); err != nil {
		return nil, "", "", "", errors.Annotate(err, "cannot extract uploaded charm")
	}

	charmDir, err := charm.ReadCharmDir(extractPath)
	if err != nil {
		return nil, "", "", "", errors.Annotate(err, "cannot read extracted charm")
	}

	// Try to get the version details here.
	// read just the first line of the file.
	var version string
	versionPath := filepath.Join(extractPath, "version")
	if file, err := os.Open(versionPath); err == nil {
		version, err = charm.ReadVersion(file)
		_ = file.Close()
		if err != nil {
			return nil, "", "", "", errors.Trace(err)
		}
	} else if !os.IsNotExist(err) {
		return nil, "", "", "", errors.Annotate(err, "cannot open version file")
	}

	// Bundle the charm and calculate its sha256 hash at the same time.
	var repackagedArchive bytes.Buffer
	hash := sha256.New()
	err = charmDir.ArchiveTo(io.MultiWriter(hash, &repackagedArchive))
	if err != nil {
		return nil, "", "", "", errors.Annotate(err, "cannot repackage uploaded charm")
	}
	archiveSHA256 := hex.EncodeToString(hash.Sum(nil))

	// Now we need to repackage it with the reserved URL, upload it to
	// provider storage and update the state.
	charmStorage := services.NewCharmStorage(services.CharmStorageConfig{
		Logger:       logger,
		StateBackend: uploader,
		ObjectStore:  objectStore,
	})

	storagePath, err := charmStorage.Store(ctx, curl, downloader.DownloadedCharm{
		Charm:        archive,
		CharmData:    &repackagedArchive,
		CharmVersion: version,
		Size:         int64(repackagedArchive.Len()),
		SHA256:       archiveSHA256,
		LXDProfile:   charmDir.LXDProfile(),
	})

	if err != nil {
		return nil, "", "", "", errors.Annotate(err, "cannot store charm")
	}

	return archive, archiveSHA256, version, storagePath, nil
}
