// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"context"
	"net/url"
	"os"

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

// DownloadClient describes the API exposed by the charmhub client.
type DownloadClient interface {
	// Download retrieves the specified charm from the store and saves its
	// contents to the specified path. Read the path to get the contents of the
	// charm.
	Download(ctx context.Context, url *url.URL, path string, options ...charmhub.DownloadOption) (*charmhub.Digest, error)
}

// DownloadResult contains information about a downloaded charm.
type DownloadResult struct {
	SHA256 string
	SHA384 string
	Path   string
	Size   int64
}

// CharmDownloader implements store-agnostic download and persistence of charm
// blobs.
type CharmDownloader struct {
	client DownloadClient
	logger logger.Logger
}

// NewCharmDownloader returns a new charm downloader instance.
func NewCharmDownloader(client DownloadClient, logger logger.Logger) *CharmDownloader {
	return &CharmDownloader{
		client: client,
		logger: logger,
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
func (d *CharmDownloader) Download(ctx context.Context, url *url.URL, hash string) (_ *DownloadResult, err error) {
	d.logger.Debugf(ctx, "downloading charm: %s", url)

	tmpFile, err := os.CreateTemp("", "charm-")
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
			d.logger.Warningf(ctx, "failed to remove temporary file %q: %v", tmpFile.Name(), removeErr)
		}
	}()

	// Force the sha256 digest to be calculated on download.
	digest, err := d.client.Download(ctx, url, tmpFile.Name())
	if err != nil {
		return nil, errors.Capture(err)
	}

	// We expect that after downloading the result is verified.
	if digest.SHA256 != hash {
		return nil, errors.Errorf("%w: %q, got %q", ErrInvalidDigestHash, hash, digest.SHA256)
	}

	d.logger.Debugf(ctx, "downloaded charm: %q", url)

	return &DownloadResult{
		SHA256: digest.SHA256,
		SHA384: digest.SHA384,
		Path:   tmpFile.Name(),
		Size:   digest.Size,
	}, nil
}
