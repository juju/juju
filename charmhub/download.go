// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/juju/charm/v9"
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

// DownloadOption to be passed to Info to customize the resulting request.
type DownloadOption func(*downloadOptions)

type downloadOptions struct {
	progressBar ProgressBar
}

// WithProgressBar sets the channel on the option.
func WithProgressBar(pb ProgressBar) DownloadOption {
	return func(options *downloadOptions) {
		options.progressBar = pb
	}
}

// Create a downloadOptions instance with default values.
func newDownloadOptions() *downloadOptions {
	return &downloadOptions{}
}

// DownloadClient represents a client for downloading charm resources directly.
type DownloadClient struct {
	transport  Transport
	fileSystem FileSystem
	logger     Logger
}

// NewDownloadClient creates a DownloadClient for requesting
func NewDownloadClient(transport Transport, fileSystem FileSystem, logger Logger) *DownloadClient {
	return &DownloadClient{
		transport:  transport,
		fileSystem: fileSystem,
		logger:     logger,
	}
}

// DownloadKey represents a key for accessing the context value.
type DownloadKey string

const (
	// DownloadNameKey defines a name of a download, so the progress bar can
	// show it.
	DownloadNameKey DownloadKey = "download-name-key"
)

// ProgressBar defines a progress bar type for giving feedback to the user about
// the state of the download.
type ProgressBar interface {
	io.Writer

	// Start progress with max "total" steps.
	Start(label string, total float64)
	// Finished the progress display
	Finished()
}

// Download returns the raw charm zip file, which is retrieved from the given
// URL.
// It is expected that the archive path doesn't already exist and if it does, it
// will error out. It is expected that the callee handles the clean up of the
// archivePath.
// TODO (stickupkid): We should either create and remove, or take a file and
// let the callee remove. The fact that the operations are asymmetrical can lead
// to unexpected expectations; namely leaking of files.
func (c *DownloadClient) Download(ctx context.Context, resourceURL *url.URL, archivePath string, options ...DownloadOption) error {
	opts := newDownloadOptions()
	for _, option := range options {
		option(opts)
	}

	f, err := c.fileSystem.Create(archivePath)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		_ = f.Close()
	}()

	r, err := c.downloadFromURL(ctx, resourceURL)
	if err != nil {
		return errors.Annotatef(err, "cannot retrieve %q", resourceURL)
	}
	defer func() {
		_ = r.Body.Close()
	}()

	var writer io.Writer = f
	if opts.progressBar != nil {
		// Progress bar has this nifty feature where you can supply a name. In
		// this case we can supply one to help with UI feedback.
		var name string
		if n := ctx.Value(DownloadNameKey); n != nil {
			if s, ok := n.(string); ok && s != "" {
				name = s
			}
		}

		// TODO (stickupkid): Would be good to verify the size, but
		// unfortunately we don't have the information to hand. That information
		// is further up the stack.
		downloadSize := float64(r.ContentLength)
		opts.progressBar.Start(name, downloadSize)
		defer opts.progressBar.Finished()

		writer = io.MultiWriter(f, opts.progressBar)
	}

	if _, err := io.Copy(writer, r.Body); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// DownloadAndRead returns a charm archive retrieved from the given URL.
func (c *DownloadClient) DownloadAndRead(ctx context.Context, resourceURL *url.URL, archivePath string, options ...DownloadOption) (*charm.CharmArchive, error) {
	err := c.Download(ctx, resourceURL, archivePath, options...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return charm.ReadCharmArchive(archivePath)
}

// DownloadAndReadBundle returns a bundle archive retrieved from the given URL.
func (c *DownloadClient) DownloadAndReadBundle(ctx context.Context, resourceURL *url.URL, archivePath string, options ...DownloadOption) (*charm.BundleArchive, error) {
	err := c.Download(ctx, resourceURL, archivePath, options...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return charm.ReadBundleArchive(archivePath)
}

// DownloadResource returns an io.ReadCloser to read the Resource from.
func (c *DownloadClient) DownloadResource(ctx context.Context, resourceURL *url.URL) (r io.ReadCloser, err error) {
	resp, err := c.downloadFromURL(ctx, resourceURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return resp.Body, nil
}

func (c *DownloadClient) downloadFromURL(ctx context.Context, resourceURL *url.URL) (resp *http.Response, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", resourceURL.String(), nil)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot make new request")
	}

	resp, err = c.transport.Do(req)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get archive")
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusNoContent {
		return resp, nil
	}

	// Clean up, as we can't really offer anything of use here.
	_, _ = io.Copy(ioutil.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.NotFoundf("archive")
	}
	return nil, errors.Errorf("unable to locate archive")
}
