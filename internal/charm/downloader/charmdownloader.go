// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/logger"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/uuid"
)

type ObjectStore interface {
	Put(ctx context.Context, path string, data io.Reader, size int64) (string, error)
}

// UniqueBlobNamer is a function that takes a charm name and returns a storage
// name.
type UniqueBlobNamer func(string) (string, error)

// CharmDownloader implements store-agnostic download and pesistence of charm
// blobs.
type CharmDownloader struct {
	repoGetter  RepositoryGetter
	objectStore ObjectStore
	namer       UniqueBlobNamer
	logger      logger.Logger
}

// NewCharmDownloader returns a new charm downloader instance.
func NewCharmDownloader(repoGetter RepositoryGetter, objectStore ObjectStore, logger logger.Logger) *CharmDownloader {
	return &CharmDownloader{
		repoGetter:  repoGetter,
		objectStore: objectStore,
		namer: func(name string) (string, error) {
			suffix, err := uuid.NewUUID()
			if err != nil {
				return "", errors.Trace(err)
			}
			return fmt.Sprintf("%s-%s", name, suffix), nil
		},
		logger: logger,
	}
}

// DownloadCharm looks up the requested charm using the appropriate store,
// downloads it to a temporary file and passes it to the configured storage API
// so it can be persisted.
//
// The method ensures that all temporary resources are cleaned up before
// returning.
func (d *CharmDownloader) DownloadCharm(ctx context.Context, name string, requestedOrigin corecharm.Origin, force bool) (string, corecharm.Origin, error) {
	channelOrigin := requestedOrigin

	var err error
	channelOrigin.Platform, err = d.normalizePlatform(name, requestedOrigin.Platform)
	if err != nil {
		return "", corecharm.Origin{}, errors.Trace(err)
	}

	// Get the repository for the requested origin source. Allowing us to
	// switch between different charm sources (charmhub).
	repo, err := d.getRepo(ctx, requestedOrigin.Source)
	if err != nil {
		return "", corecharm.Origin{}, errors.Trace(err)
	}

	// Download the charm from the repository and hash it.
	downloadedCharm, actualOrigin, err := d.downloadAndHash(ctx, name, channelOrigin, repo)
	if err != nil {
		return "", corecharm.Origin{}, errors.Annotatef(err, "downloading charm %q from origin %v", name, requestedOrigin)
	}

	// Grab a unique name for the charm blob. This can be totally random, we
	// just need to ensure that it is unique. The name is the prefix to see
	// the charm in the storage, but eventually, we don't need this.
	storageName, err := d.namer(name)
	if err != nil {
		return "", corecharm.Origin{}, errors.Trace(err)
	}

	// Put the charm in the blob store. We don't need to store the charm
	// directly.
	uuid, err := d.objectStore.Put(ctx, storageName, downloadedCharm.CharmData, downloadedCharm.Size)
	if err != nil && !errors.Is(err, objectstoreerrors.ErrHashAlreadyExists) {
		return "", corecharm.Origin{}, errors.Annotate(err, "cannot add charm to storage")
	}

	return uuid, actualOrigin, nil
}

func (d *CharmDownloader) getRepo(ctx context.Context, src corecharm.Source) (CharmRepository, error) {
	repo, err := d.repoGetter.GetCharmRepository(ctx, src)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return repo, nil
}

func (d *CharmDownloader) downloadAndHash(ctx context.Context, name string, requestedOrigin corecharm.Origin, repo CharmRepository) (*DownloadedCharm, corecharm.Origin, error) {
	d.logger.Debugf("downloading charm %q from requested origin %v", name, requestedOrigin)

	// Download charm blob to a temp file
	tmpFile, err := os.CreateTemp("", name)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}
	defer func() {
		_ = tmpFile.Close()
		if err := os.Remove(tmpFile.Name()); err != nil {
			d.logger.Warningf("unable to remove temporary charm download path %q", tmpFile.Name())
		}
	}()

	chArchive, actualOrigin, err := repo.DownloadCharm(ctx, name, requestedOrigin, tmpFile.Name())
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}
	d.logger.Debugf("downloaded charm %q from actual origin %v", name, actualOrigin)

	return &DownloadedCharm{
		Charm:        chArchive,
		CharmVersion: chArchive.Version(),
		Size:         size,
		LXDProfile:   chArchive.LXDProfile(),
		SHA256:       sha,
	}, actualOrigin, nil
}

func (d *CharmDownloader) normalizePlatform(charmName string, platform corecharm.Platform) (corecharm.Platform, error) {
	arc := platform.Architecture
	if platform.Architecture == "" || platform.Architecture == "all" {
		d.logger.Warningf("received charm Architecture: %q, changing to %q, for charm %q", platform.Architecture, arch.DefaultArchitecture, charmName)
		arc = arch.DefaultArchitecture
	}

	// Values passed to the api are case sensitive: ubuntu succeeds and
	// Ubuntu returns `"code": "revision-not-found"`
	return corecharm.Platform{
		Architecture: arc,
		OS:           strings.ToLower(platform.OS),
		Channel:      platform.Channel,
	}, nil
}
