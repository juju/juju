// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/logger"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/charm"
	charmdownloader "github.com/juju/juju/internal/charm/downloader"
	"github.com/juju/juju/internal/uuid"
)

// CharmStorageConfig encapsulates the information required for creating a
// new CharmStorage instance.
type CharmStorageConfig struct {
	// The logger to use.
	Logger logger.Logger

	// A factory for accessing model-scoped storage for charm blobs.
	ObjectStore Storage

	ApplicationService ApplicationService
}

// CharmStorage provides an abstraction for storing charm blobs.
type CharmStorage struct {
	logger             logger.Logger
	objectStore        Storage
	applicationService ApplicationService
	uuidGenerator      func() (uuid.UUID, error)
}

// NewCharmStorage creates a new CharmStorage instance with the specified config.
func NewCharmStorage(cfg CharmStorageConfig) *CharmStorage {
	return &CharmStorage{
		logger:             cfg.Logger,
		objectStore:        cfg.ObjectStore,
		uuidGenerator:      uuid.NewUUID,
		applicationService: cfg.ApplicationService,
	}
}

// PrepareToStoreCharm ensures that the store is ready to process the specified
// charm URL. If the blob for the charm is already stored, the method returns
// an error to indicate this.
func (s *CharmStorage) PrepareToStoreCharm(charmURL string) error {
	parsedURL, err := charm.ParseURL(charmURL)
	if err != nil {
		return errors.Trace(err)
	}
	parsedSource, err := applicationcharm.ParseCharmSchema(charm.Schema(parsedURL.Schema))
	if err != nil {
		return errors.Trace(err)
	}
	charmID, err := s.applicationService.GetCharmID(context.Background(), applicationcharm.GetCharmArgs{
		Name:     parsedURL.Name,
		Revision: &parsedURL.Revision,
		Source:   parsedSource,
	})
	if err != nil {
		return errors.Trace(err)
	}
	_, _, err = s.applicationService.GetCharm(context.Background(), charmID)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// CharmStorage attempts to store the contents of a downloaded charm.
func (s *CharmStorage) Store(ctx context.Context, charmURL string, downloadedCharm charmdownloader.DownloadedCharm) (string, error) {
	s.logger.Tracef("store %q", charmURL)
	storagePath, err := s.charmArchiveStoragePath(charmURL)
	if err != nil {
		return "", errors.Annotate(err, "cannot generate charm archive name")
	}

	// If the blob is already stored, we can skip the upload.
	if _, err := s.objectStore.Put(ctx, storagePath, downloadedCharm.CharmData, downloadedCharm.Size); err != nil && !errors.Is(err, objectstoreerrors.ErrPathAlreadyExistsDifferentHash) {
		return "", errors.Annotate(err, "cannot add charm to storage")
	}

	return storagePath, nil
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
