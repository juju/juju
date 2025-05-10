// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"context"
	"io"
	"net/http"
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils/v4"

	corelogger "github.com/juju/juju/core/logger"
	jujuhttp "github.com/juju/juju/internal/http"
)

// NewHTTPBlobOpener returns a blob opener func suitable for use with
// Download. The opener func uses an HTTP client that enforces the
// provided SSL hostname verification policy.
func NewHTTPBlobOpener(hostnameVerification bool) func(Request) (io.ReadCloser, error) {
	return func(req Request) (io.ReadCloser, error) {
		// TODO(rog) make the download operation interruptible.
		client := jujuhttp.NewClient(
			jujuhttp.WithSkipHostnameVerification(!hostnameVerification),
			jujuhttp.WithLogger(logger.Child("http", corelogger.HTTP)),
		)

		resp, err := client.Get(context.Background(), req.URL.String())
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			// resp.Body is always non-nil. (see https://golang.org/pkg/net/http/#Response)
			_ = resp.Body.Close()

			// Blob is pending to be downloaded
			if resp.StatusCode == http.StatusConflict {
				return nil, errors.NotYetAvailablef("blob contents")
			}
			return nil, errors.Errorf("bad http response: %v", resp.Status)
		}
		return resp.Body, nil
	}
}

// NewSha256Verifier returns a verifier suitable for Request. The
// verifier checks the SHA-256 checksum of the file to ensure that it
// matches the one returned by the provided func.
func NewSha256Verifier(expected string) func(*os.File) error {
	return func(file *os.File) error {
		actual, _, err := utils.ReadSHA256(file)
		if err != nil {
			return errors.Trace(err)
		}
		if actual != expected {
			err := errors.Errorf("expected sha256 %q, got %q", expected, actual)
			return errors.NewNotValid(err, "")
		}
		return nil
	}
}
