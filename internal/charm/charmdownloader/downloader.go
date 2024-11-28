// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"context"
	"net/url"
	"os"
	"strings"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/errors"
)

const (
	// ErrInvalidDigestHash is returned when the hash of the downloaded charm
	// does not match the expected hash.
	ErrInvalidDigestHash = errors.ConstError("invalid origin hash")

	// ErrInvalidOriginHash is returned when the hash of the actual origin
	// does not match the expected hash.
	ErrInvalidOriginHash = errors.ConstError("invalid origin hash")
)

// CharmhubClient describes the API exposed by the charmhub client.
type CharmhubClient interface {
	// Download retrieves the specified charm from the store and saves its
	// contents to the specified path. Read the path to get the contents of the
	// charm.
	Download(ctx context.Context, url *url.URL, path string, options ...charmhub.DownloadOption) (*charmhub.Digest, error)
}

// CharmRepository provides an API for downloading charms/bundles.
type CharmRepository interface {
	// GetDownloadURL returns the url from which to download the CharmHub
	// charm/bundle defined by the provided charm name and origin.  An updated
	// charm origin is also returned with the ID and hash for the charm to be
	// downloaded. If the provided charm origin has no ID, it is assumed that
	// the charm is being installed, not refreshed.
	GetDownloadURL(ctx context.Context, charmName string, origin corecharm.Origin) (*url.URL, corecharm.Origin, error)
}

// DownloadResult contains information about a downloaded charm.
type DownloadResult struct {
	Path   string
	Origin corecharm.Origin
	Size   int64
}

// CharmDownloader implements store-agnostic download and persistence of charm
// blobs.
type CharmDownloader struct {
	repository CharmRepository
	client     CharmhubClient
	logger     logger.Logger
}

// NewCharmDownloader returns a new charm downloader instance.
func NewCharmDownloader(repository CharmRepository, client CharmhubClient, logger logger.Logger) *CharmDownloader {
	return &CharmDownloader{
		repository: repository,
		client:     client,
		logger:     logger,
	}
}

// Download looks up the requested charm using the appropriate store, downloads
// it to a temporary file and passes it to the configured storage API so it can
// be persisted.
//
// The resulting charm is verified to be the right hash. It expected that the
// origin will always have the correct hash following this call.
//
// Returns [ErrInvalidHash] if the hash of the downloaded charm does not match
// the expected hash.
func (d *CharmDownloader) Download(ctx context.Context, name string, requestedOrigin corecharm.Origin) (*DownloadResult, error) {
	channelOrigin := requestedOrigin

	var err error
	channelOrigin.Platform, err = d.normalizePlatform(name, requestedOrigin.Platform)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Download the charm from the repository and hash it.
	result, err := d.download(ctx, name, channelOrigin)
	if err != nil {
		return nil, errors.Errorf("downloading %q using origin %v: %w", name, requestedOrigin, err)
	}

	return result, nil
}

func (d *CharmDownloader) download(ctx context.Context, name string, origin corecharm.Origin) (result *DownloadResult, err error) {
	d.logger.Debugf("downloading charm %q using origin %v", name, origin)

	// Resolve charm URL to a link to the charm blob and keep track of the
	// actual resolved origin which may be different from the requested one.
	url, downloadOrigin, err := d.repository.GetDownloadURL(ctx, name, origin)
	if err != nil {
		return nil, errors.Capture(err)
	}

	tmpFile, err := os.CreateTemp("", name)
	if err != nil {
		return nil, errors.Capture(err)
	}

	defer func() {
		// Always close the file, we no longer require to have it open for
		// this purpose. Another process/method can take over the file.
		tmpFile.Close()

		// If the download was successful, we don't need to remove the file.
		// It is the responsibility of the caller to remove the file.
		if err == nil {
			return
		}

		// Remove the temporary file if the download failed. If we can't
		// remove the file, log a warning, so the operator can clean it up
		// manually.
		removeErr := os.Remove(tmpFile.Name())
		if removeErr != nil {
			d.logger.Warningf("failed to remove temporary file %q: %v", tmpFile.Name(), removeErr)
		}
	}()

	// Force the sha256 digest to be calculated on download.
	digest, err := d.client.Download(ctx, url, tmpFile.Name(), charmhub.WithEnsureDigest(charmhub.SHA256))
	if err != nil {
		return nil, errors.Capture(err)
	}

	// We expect that after downloading the result is verified.
	if digest.Hash != origin.Hash {
		return nil, errors.Errorf("%w: %q, got %q", ErrInvalidDigestHash, origin.Hash, digest.Hash)
	} else if downloadOrigin.Hash != origin.Hash {
		return nil, errors.Errorf("%w: %q, got %q", ErrInvalidOriginHash, origin.Hash, downloadOrigin.Hash)
	}

	d.logger.Debugf("downloaded charm %q from actual origin %v", name, downloadOrigin)

	return &DownloadResult{
		Path:   tmpFile.Name(),
		Origin: downloadOrigin,
		Size:   digest.Size,
	}, nil
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
