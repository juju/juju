// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"context"
	"io"
	"net/url"
	"os"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/errors"
)

const (
	// ErrUnexpectedHash is returned when the hash of the downloaded resource
	// does not match the expected hash.
	ErrUnexpectedHash = errors.ConstError("downloaded resource has unexpected hash")

	// ErrUnexpectedSize is returned when the size of the downloaded resources
	// does not match the expected size.
	ErrUnexpectedSize = errors.ConstError("downloaded resource has unexpected size")
)

// DownloadClient describes the API exposed by the charmhub client.
type DownloadClient interface {
	// Download retrieves the specified resource from the store and saves its
	// contents to the specified path. Read the path to get the contents of the
	// resource.
	Download(ctx context.Context, url *url.URL, path string, options ...charmhub.DownloadOption) (*charmhub.Digest, error)
}

// ResourceDownloader implements store-agnostic download and persistence of charm
// blobs.
type ResourceDownloader struct {
	client DownloadClient
	logger logger.Logger
}

// NewResourceDownloader returns a new charm downloader instance.
func NewResourceDownloader(client DownloadClient, logger logger.Logger) *ResourceDownloader {
	return &ResourceDownloader{
		client: client,
		logger: logger,
	}
}

// Download looks up the requested resource using the appropriate store,
// downloads it to a temporary file and returns a ReadCloser that deletes the
// temporary file on closure.
//
// The resulting resource is verified to have the right hash and size.
//
// Returns [ErrUnexpectedHash] if the hash of the downloaded resource does not
// match the expected hash.
// Returns [ErrUnexpectedSize] if the size of the downloaded resource does not
// match the expected size.
func (d *ResourceDownloader) Download(
	ctx context.Context,
	url *url.URL,
	sha384 string,
	size int64,
) (io.ReadCloser, error) {
	tmpFile, err := os.CreateTemp("", "resource-")
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

	d.logger.Debugf(ctx, "downloading resource: %s", url)

	// Force the sha256 and sha384 digest to be calculated on download.
	digest, err := d.client.Download(ctx, url, tmpFile.Name())
	if err != nil {
		return nil, errors.Capture(err)
	}

	d.logger.Debugf(ctx, "downloaded resource: %s", url)

	if digest.SHA384 != sha384 {
		return nil, errors.Errorf(
			"%w: %q, got %q", ErrUnexpectedHash, sha384, digest.SHA384,
		)
	}
	if digest.Size != size {
		return nil, errors.Errorf(
			"%w: %q, got %q", ErrUnexpectedSize, size, digest.Size,
		)
	}

	// Create a reader for the temporary file containing the resource.
	tmpFileReader, err := newTmpFileReader(tmpFile.Name(), d.logger)
	if err != nil {
		return nil, errors.Errorf("opening downloaded resource: %w", err)
	}

	return tmpFileReader, nil
}

// tmpFileReader wraps an *os.File and deletes it when closed.
type tmpFileReader struct {
	logger logger.Logger
	*os.File
}

func newTmpFileReader(file string, logger logger.Logger) (*tmpFileReader, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	return &tmpFileReader{
		logger: logger,
		File:   f,
	}, nil
}

// Close closes the temporary file and removes it. If the file cannot be
// removed, an error is logged.
func (f *tmpFileReader) Close() (err error) {
	defer func() {
		removeErr := os.Remove(f.Name())
		if err == nil {
			err = removeErr
		} else if removeErr != nil {
			f.logger.Warningf(context.Background(), "failed to remove temporary file %q: %v", f.Name(), removeErr)
		}
	}()

	return f.File.Close()
}
