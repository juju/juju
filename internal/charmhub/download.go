// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime/pprof"

	"github.com/juju/errors"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/trace"
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

// DefaultFileSystem returns the default file system.
func DefaultFileSystem() FileSystem {
	return fileSystem{}
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

// Digest represents a digest of a file.
type Digest struct {
	SHA256 string
	SHA384 string
	Size   int64
}

// Create a downloadOptions instance with default values.
func newDownloadOptions() *downloadOptions {
	return &downloadOptions{}
}

// DownloadClient represents a client for downloading charm resources directly.
type DownloadClient struct {
	httpClient HTTPClient
	fileSystem FileSystem
	logger     corelogger.Logger
}

// newDownloadClient creates a DownloadClient for requesting
func NewDownloadClient(httpClient HTTPClient, fileSystem FileSystem, logger corelogger.Logger) *DownloadClient {
	return &DownloadClient{
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
func (c *DownloadClient) Download(ctx context.Context, resourceURL *url.URL, archivePath string, options ...DownloadOption) (digest *Digest, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(), trace.WithAttributes(
		trace.StringAttr("charmhub.request", "download"),
		trace.StringAttr("charmhub.url", resourceURL.String()),
	))
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	pprof.Do(ctx, pprof.Labels(trace.OTELTraceID, span.Scope().TraceID()), func(ctx context.Context) {
		digest, err = c.download(ctx, resourceURL, archivePath, options...)
	})
	return
}

func (c *DownloadClient) download(ctx context.Context, url *url.URL, archivePath string, options ...DownloadOption) (*Digest, error) {
	opts := newDownloadOptions()
	for _, option := range options {
		option(opts)
	}

	f, err := c.fileSystem.Create(archivePath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		_ = f.Close()
	}()

	r, err := c.downloadFromURL(ctx, url)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot retrieve %q", url)
	}
	defer func() {
		_ = r.Body.Close()
	}()

	progressBar := io.Discard
	if opts.progressBar != nil {
		// Progress bar has this nifty feature where you can supply a name. In
		// this case we can supply one to help with UI feedback.
		var name string
		if n := ctx.Value(DownloadNameKey); n != nil {
			if s, ok := n.(string); ok && s != "" {
				name = s
			}
		}

		downloadSize := float64(r.ContentLength)
		opts.progressBar.Start(name, downloadSize)
		defer opts.progressBar.Finished()

		progressBar = opts.progressBar
	}

	hasher256 := sha256.New()
	hasher384 := sha512.New384()

	size, err := io.Copy(f, io.TeeReader(r.Body, io.MultiWriter(hasher256, hasher384, progressBar)))
	if err != nil {
		return nil, errors.Trace(err)
	} else if size != r.ContentLength {
		return nil, errors.Errorf("downloaded size %d does not match expected size %d", size, r.ContentLength)
	}

	return &Digest{
		SHA256: hex.EncodeToString(hasher256.Sum(nil)),
		SHA384: hex.EncodeToString(hasher384.Sum(nil)),
		Size:   size,
	}, nil
}

func (c *DownloadClient) downloadFromURL(ctx context.Context, resourceURL *url.URL) (resp *http.Response, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", resourceURL.String(), nil)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot make new request")
	}

	c.logger.Tracef(ctx, "download from URL %s", resourceURL.String())

	resp, err = c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get archive")
	}
	// If we get anything but a 200 status code, we don't know how to correctly
	// handle that scenario. Return early and deal with the failure later on.
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}

	c.logger.Errorf(ctx, "download failed from %s: response code: %s", resourceURL.String(), resp.Status)

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
