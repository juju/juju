// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
)

// FileSystem defines a file system for modifying files on a users system.
type FileSystem interface {
	// Create creates or truncates the named file. If the file already exists,
	// it is truncated.
	Create(string) (*os.File, error)
}

// DefaultFileSystem is the file system used for most download requests.
func DefaultFileSystem() FileSystem {
	return fileSystem{}
}

type fileSystem struct{}

// Create creates or truncates the named file. If the file already exists,
// it is truncated.
func (fileSystem) Create(name string) (*os.File, error) {
	return os.Create(name)
}

// DownloadClient represents a client for downloading charm resources directly.
type DownloadClient struct {
	transport  Transport
	fileSystem FileSystem
}

// NewDownloadClient creates a DownloadClient for requesting
func NewDownloadClient(transport Transport, fileSystem FileSystem) *DownloadClient {
	return &DownloadClient{
		transport:  transport,
		fileSystem: fileSystem,
	}
}

// Download returns a charm archive retrieved from the given URL.
func (c *DownloadClient) Download(ctx context.Context, resourceURL *url.URL, archivePath string) (*charm.CharmArchive, error) {
	f, err := c.fileSystem.Create(archivePath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		_ = f.Close()
	}()

	r, err := c.downloadFromURL(ctx, resourceURL)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot retrieve %q", resourceURL)
	}
	defer func() {
		_ = r.Close()
	}()

	// TODO (stickupkid): Would be good to verify the size, but unfortunately
	// we don't have the information to hand. That information is further up the
	// stack.
	if _, err := io.Copy(f, r); err != nil {
		return nil, errors.Trace(err)
	}

	return charm.ReadCharmArchive(archivePath)
}

func (c *DownloadClient) downloadFromURL(ctx context.Context, resourceURL *url.URL) (r io.ReadCloser, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", resourceURL.String(), nil)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot make new request")
	}

	resp, err := c.transport.Do(req)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get archive")
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusNoContent {
		return resp.Body, nil
	}
	defer func() { _ = resp.Body.Close() }()

	return nil, errors.Errorf("unable to locate archive")
}
