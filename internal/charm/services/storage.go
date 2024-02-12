// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	charmdownloader "github.com/juju/juju/core/charm/downloader"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
)

// CharmStorageConfig encapsulates the information required for creating a
// new CharmStorage instance.
type CharmStorageConfig struct {
	// The logger to use.
	Logger Logger

	// A factory for accessing model-scoped storage for charm blobs.
	ObjectStore Storage

	StateBackend StateBackend
}

// CharmStorage provides an abstraction for storing charm blobs.
type CharmStorage struct {
	logger        Logger
	stateBackend  StateBackend
	objectStore   Storage
	uuidGenerator func() (uuid.UUID, error)
}

// NewCharmStorage creates a new CharmStorage instance with the specified config.
func NewCharmStorage(cfg CharmStorageConfig) *CharmStorage {
	return &CharmStorage{
		logger:        cfg.Logger,
		stateBackend:  cfg.StateBackend,
		objectStore:   cfg.ObjectStore,
		uuidGenerator: uuid.NewUUID,
	}
}

// PrepareToStoreCharm ensures that the store is ready to process the specified
// charm URL. If the blob for the charm is already stored, the method returns
// an error to indicate this.
func (s *CharmStorage) PrepareToStoreCharm(charmURL string) error {
	ch, err := s.stateBackend.PrepareCharmUpload(charmURL)
	if err != nil {
		return errors.Trace(err)
	}

	if ch.IsUploaded() {
		return charmdownloader.NewCharmAlreadyStoredError(charmURL)
	}

	return nil
}

// CharmStorage attempts to store the contents of a downloaded charm.
func (s *CharmStorage) Store(ctx context.Context, charmURL string, downloadedCharm charmdownloader.DownloadedCharm) error {
	s.logger.Tracef("store %q", charmURL)
	storagePath, err := s.charmArchiveStoragePath(charmURL)
	if err != nil {
		return errors.Annotate(err, "cannot generate charm archive name")
	}
	if err := s.objectStore.Put(ctx, storagePath, downloadedCharm.CharmData, downloadedCharm.Size); err != nil {
		return errors.Annotate(err, "cannot add charm to storage")
	}

	info := state.CharmInfo{
		StoragePath: storagePath,
		Charm:       downloadedCharm.Charm,
		ID:          charmURL,
		SHA256:      downloadedCharm.SHA256,
		Version:     downloadedCharm.CharmVersion,
	}

	// Now update the charm data in state and mark it as no longer pending.
	_, err = s.stateBackend.UpdateUploadedCharm(info)
	if err != nil {
		alreadyUploaded := err == stateerrors.ErrCharmRevisionAlreadyModified ||
			errors.Cause(err) == stateerrors.ErrCharmRevisionAlreadyModified ||
			stateerrors.IsCharmAlreadyUploadedError(err)
		if err := s.objectStore.Remove(ctx, storagePath); err != nil {
			if alreadyUploaded {
				s.logger.Errorf("cannot remove duplicated charm archive from storage: %v", err)
			} else {
				s.logger.Errorf("cannot remove unsuccessfully recorded charm archive from storage: %v", err)
			}
		}
		if alreadyUploaded {
			// Somebody else managed to upload and update the charm in
			// state before us. This is not an error.
			return nil
		}
		return errors.Trace(err)
	}
	return nil
}

// charmArchiveStoragePath returns a string that is suitable as a
// storage path, using a random UUID to avoid colliding with concurrent
// uploads.
func (s *CharmStorage) charmArchiveStoragePath(charmURL string) (string, error) {
	uuid, err := s.uuidGenerator()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("charms/%s-%s", charmURL, uuid), nil
}
