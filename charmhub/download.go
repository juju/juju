// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
)

// FileSystem defines a file system for modifying files on a users system.
type FileSystem interface {
	// Create creates or truncates the named file. If the file already exists,
	// it is truncated.
	Create(string) (*os.File, error)
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

// downloadClient represents a client for downloading charm resources directly.
type downloadClient struct {
	httpClient HTTPClient
	fileSystem FileSystem
	logger     Logger
}

// newDownloadClient creates a downloadClient for requesting
func newDownloadClient(httpClient HTTPClient, fileSystem FileSystem, logger Logger) *downloadClient {
	return &downloadClient{
		httpClient: httpClient,
		fileSystem: fileSystem,
		logger:     logger,
	}
}

// downloadKey represents a key for accessing the context value.
type downloadKey string

const (
	// DownloadNameKey defines a name of a download, so the progress bar can
	// show it.
	DownloadNameKey downloadKey = "download-name-key"
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
func (c *downloadClient) Download(ctx context.Context, resourceURL *url.URL, archivePath string, options ...DownloadOption) error {
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
func (c *downloadClient) DownloadAndRead(ctx context.Context, resourceURL *url.URL, archivePath string, options ...DownloadOption) (*charm.CharmArchive, error) {
	err := c.Download(ctx, resourceURL, archivePath, options...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return charm.ReadCharmArchive(archivePath)
}

// DownloadAndReadBundle returns a bundle archive retrieved from the given URL.
func (c *downloadClient) DownloadAndReadBundle(ctx context.Context, resourceURL *url.URL, archivePath string, options ...DownloadOption) (*charm.BundleArchive, error) {
	err := c.Download(ctx, resourceURL, archivePath, options...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return charm.ReadBundleArchive(archivePath)
}

// DownloadResource returns an io.ReadCloser to read the Resource from.
func (c *downloadClient) DownloadResource(ctx context.Context, resourceURL *url.URL) (r io.ReadCloser, err error) {
	resp, err := c.downloadFromURL(ctx, resourceURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return resp.Body, nil
}

func (c *downloadClient) downloadFromURL(ctx context.Context, resourceURL *url.URL) (resp *http.Response, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", resourceURL.String(), nil)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot make new request")
	}

	c.logger.Tracef("download from URL %s", resourceURL.String())

	resp, err = c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get archive")
	}
	// If we get anything but a 200 status code, we don't know how to correctly
	// handle that scenario. Return early and deal with the failure later on.
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}

	c.logger.Errorf("download failed from %s: response code: %s", resourceURL.String(), resp.Status)

	// Ensure we drain the response body so this connection can be reused. As
	// there is no error message, we have no ability other than to check the
	// status codes.
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.NotFoundf("archive")
	}

	// Server error, nothing we can do other than inform the user that the
	// archive was unaviable.
	return nil, errors.Errorf("unable to locate archive (store API responded with status: %s)", resp.Status)
}
