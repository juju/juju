// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"io"
	"net/url"
	"os"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.downloader")

// Downloader provides the functionality for downloading files.
type Downloader struct {
	// OpenBlob is the func used to gain access to the blob, whether
	// through an HTTP request or some other means.
	OpenBlob func(*url.URL) (io.ReadCloser, error)
}

// NewArgs holds the arguments to New().
type NewArgs struct {
	// HostnameVerification is that which should be used for the client.
	// If it is disableSSLHostnameVerification then a non-validating
	// client will be used.
	HostnameVerification bool
}

// New returns a new Downloader for the given args.
func New(args NewArgs) *Downloader {
	return &Downloader{
		OpenBlob: NewHTTPBlobOpener(args.HostnameVerification),
	}
}

// Start starts a new download and returns it.
func (dlr Downloader) Start(req Request) *Download {
	return StartDownload(req, dlr.OpenBlob)
}

// Download starts a new download, waits for it to complete, and
// returns the local name of the file. The download can be aborted by
// closing the Abort channel in the Request provided.
func (dlr Downloader) Download(req Request) (string, error) {
	if err := os.MkdirAll(req.TargetDir, 0755); err != nil {
		return "", errors.Trace(err)
	}
	dl := dlr.Start(req)
	filename, err := dl.Wait()
	return filename, errors.Trace(err)
}
