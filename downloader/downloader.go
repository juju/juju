// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"io"
	"net/url"
	"os"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
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
	HostnameVerification utils.SSLHostnameVerification
}

// New returns a new Downloader for the given args.
func New(args NewArgs) *Downloader {
	return &Downloader{
		OpenBlob: NewHTTPBlobOpener(args.HostnameVerification),
	}
}

// Start starts a new download and returns it.
func (dlr Downloader) Start(req Request) *Download {
	dl := StartDownload(req, dlr.OpenBlob)
	return dl
}

// Download starts a new download, waits for it to complete, and
// returns the local name of the file.
func (dlr Downloader) Download(req Request, abort <-chan struct{}) (filename string, err error) {
	if err := os.MkdirAll(req.TargetDir, 0755); err != nil {
		return "", errors.Trace(err)
	}
	dl := dlr.Start(req)
	file, err := dl.Wait(abort)
	if file != nil {
		defer file.Close()
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	return file.Name(), nil
}

// DownloadWithAlternates tries each of the provided requests until
// one succeeds. If none succeed then the error from the most recent
// attempt is returned. At least one request must be provided.
func (dlr Downloader) DownloadWithAlternates(requests []Request, abort <-chan struct{}) (filename string, err error) {
	if len(requests) == 0 {
		return "", errors.New("no requests to try")
	}

	for _, req := range requests {
		filename, err = dlr.Download(req, abort)
		if errors.IsNotValid(err) {
			break
		}
		if err == nil {
			break
		}
		logger.Errorf("download request to %s failed: %v", req.URL, err)
		// Try the next one.
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	return filename, nil
}
