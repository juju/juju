// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils"
)

// NewHTTPBlobOpener returns a blob opener func suitable for use with
// Download. The opener func uses an HTTP client that enforces the
// provided SSL hostname verification policy.
func NewHTTPBlobOpener(hostnameVerification utils.SSLHostnameVerification) func(*url.URL) (io.ReadCloser, error) {
	return func(url *url.URL) (io.ReadCloser, error) {
		// TODO(rog) make the download operation interruptible.
		client := utils.GetHTTPClient(hostnameVerification)
		resp, err := client.Get(url.String())
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, errors.Errorf("bad http response: %v", resp.Status)
		}
		return resp.Body, nil
	}
}

// NewSha256Verifier returns a verifier suitable for Request. The
// verifier checks the SHA-256 checksum of the file to ensure that it
// matches the provided one.
func NewSha256Verifier(expected string) func(*os.File) error {
	return func(file *os.File) error {
		actual, _, err := utils.ReadSHA256(file)
		if err != nil {
			return errors.Trace(err)
		}
		if actual != expected {
			return errors.Errorf("expected sha256 %q, got %q", expected, actual)
		}
		return nil
	}
}
