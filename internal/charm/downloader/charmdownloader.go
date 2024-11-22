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

// ObjectStore is an interface for storing objects.
type ObjectStore interface {
	// Put stores the data in the object store at the specified path.
	// The resulting UUID can be used as RI against the object store.
	Put(ctx context.Context, path string, data io.Reader, size int64) (string, error)
}

// UniqueBlobNamer is a function that takes a charm name and returns a storage
// name.
type UniqueBlobNamer func(string) (string, error)

// DownloadUUID is a unique identifier for a downloaded charm that relates
// to the object store.
type DownloadUUID string

// Validate checks that the download UUID is a valid UUID.
func (u DownloadUUID) Validate() error {
	if !uuid.IsValidUUIDString(string(u)) {
		return errors.Errorf("invalid download UUID %q", u)
	}
	return nil
}

func (u DownloadUUID) String() string {
	return string(u)
}

// CharmDownloader implements store-agnostic download and persistence of charm
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

// DownloadAndStore looks up the requested charm using the appropriate store,
// downloads it to a temporary file and passes it to the configured storage API
// so it can be persisted.
//
// The method ensures that all temporary resources are cleaned up before
// returning.
//
// The resulting UUID can be used as RI against the object store.
func (d *CharmDownloader) DownloadAndStore(ctx context.Context, name string, requestedOrigin corecharm.Origin, force bool) (DownloadUUID, corecharm.Origin, error) {
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
	result, err := d.download(ctx, name, channelOrigin, repo)
	if err != nil {
		return "", corecharm.Origin{}, errors.Annotatef(err, "downloading %q using origin %v", name, requestedOrigin)
	}

	return d.store(ctx, name, result)
}

func (d *CharmDownloader) getRepo(ctx context.Context, src corecharm.Source) (CharmRepository, error) {
	repo, err := d.repoGetter.GetCharmRepository(ctx, src)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return repo, nil
}

func (d *CharmDownloader) download(ctx context.Context, name string, requestedOrigin corecharm.Origin, repo CharmRepository) (*downloadResult, error) {
	d.logger.Debugf("downloading charm %q using origin %v", name, requestedOrigin)

	// Create a new temporary file to store the charm.
	tmpFile, err := newTempFile(name, d.logger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Do not close or defer close the file in this function, as a successful
	// download will return the file to the caller.

	actualOrigin, digest, err := repo.Download(ctx, name, requestedOrigin, tmpFile.Name())
	if err != nil {
		// Ensure we cleanup if the download fails.
		_ = tmpFile.Close()

		return nil, errors.Trace(err)
	}

	d.logger.Debugf("downloaded charm %q from actual origin %v", name, actualOrigin)

	return &downloadResult{
		Reader: tmpFile,
		Origin: actualOrigin,
		Size:   digest.Size,
	}, nil
}

func (d *CharmDownloader) store(ctx context.Context, name string, result *downloadResult) (DownloadUUID, corecharm.Origin, error) {
	// Grab a unique name for the charm blob. This can be totally random, we
	// just need to ensure that it is unique. The name is the prefix to see
	// the charm in the storage, but eventually, we don't need this.
	storageName, err := d.namer(name)
	if err != nil {
		return "", corecharm.Origin{}, errors.Trace(err)
	}

	// Put the charm in the blob store. We don't need to store the charm
	// directly.
	//
	// Do not attempt to stream the data directly from the download, directly
	// into the object store. The hash might not be correct, and attempting to
	// evict the charm from the object store will incur costs if it's hosted
	// on a cloud provider.
	uuid, err := d.objectStore.Put(ctx, storageName, result.Reader, result.Size)
	if err != nil && !errors.Is(err, objectstoreerrors.ErrHashAlreadyExists) {
		return "", corecharm.Origin{}, errors.Annotate(err, "adding to object store")
	}

	// Close the reader now the data has been stored.
	defer result.Reader.Close()

	downloadUUID := DownloadUUID(uuid)
	if err := downloadUUID.Validate(); err != nil {
		return "", corecharm.Origin{}, errors.Trace(err)
	}

	d.logger.Debugf("stored charm %q with UUID %q", name, uuid)

	return downloadUUID, result.Origin, nil
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

type downloadResult struct {
	Reader io.ReadCloser
	Origin corecharm.Origin
	Size   int64
}

type tmpFileCloser struct {
	reader io.ReadCloser
	path   string
	logger logger.Logger
}

func newTempFile(name string, logger logger.Logger) (*tmpFileCloser, error) {
	tmpFile, err := os.CreateTemp("", name)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &tmpFileCloser{
		reader: tmpFile,
		path:   tmpFile.Name(),
		logger: logger,
	}, nil
}

func (t *tmpFileCloser) Name() string {
	return t.path
}

func (t *tmpFileCloser) Read(p []byte) (n int, err error) {
	return t.reader.Read(p)
}

func (t *tmpFileCloser) Close() error {
	err := t.reader.Close()

	// Remove the temporary file, even if it fails to close the reader.
	if removeErr := os.Remove(t.path); removeErr != nil {
		t.logger.Warningf("unable to remove temporary download path %q: %v", t.path, removeErr)
	}

	return err
}
