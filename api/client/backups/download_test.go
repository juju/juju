// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

type downloadSuite struct {
	baseSuite
}

func TestDownloadSuite(t *testing.T) {
	tc.Run(t, &downloadSuite{})
}

func (s *downloadSuite) TestDownload(c *tc.C) {
	defer s.setupMocks(c).Finish()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.URL.String(), tc.Equals, "/backups")
		_, err := w.Write([]byte("success"))
		c.Assert(err, tc.ErrorIsNil)
	}))
	defer srv.Close()
	httpClient := &httprequest.Client{BaseURL: srv.URL}
	s.apiCaller.EXPECT().HTTPClient(base.HTTPClientScopeUnscoped).Return(httpClient, nil)

	client := s.newClient()
	rdr, err := client.Download(c.Context(), "/path/to/backup")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rdr.Close() }()

	data, err := io.ReadAll(rdr)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, "success")
}

// TestDownloadNotFound verifies that the endpoint's JSON not-found
// error, as emitted by apiserver/backups.go:sendError for an unknown
// or already-served id, is classified as NotFound on the client. The
// create-backup retry logic depends on this classification to stop
// retrying a spent id.
func (s *downloadSuite) TestDownloadNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := json.Marshal(params.ErrorResult{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: `backup "backup-id" not found`,
			},
		})
		c.Assert(err, tc.ErrorIsNil)
		w.Header().Set("Content-Type", params.ContentTypeJSON)
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.WriteHeader(http.StatusNotFound)
		_, err = w.Write(body)
		c.Assert(err, tc.ErrorIsNil)
	}))
	defer srv.Close()
	httpClient := &httprequest.Client{
		BaseURL:        srv.URL,
		UnmarshalError: api.UnmarshalHTTPErrorResponse,
	}
	s.apiCaller.EXPECT().HTTPClient(base.HTTPClientScopeUnscoped).Return(httpClient, nil)

	client := s.newClient()
	rdr, err := client.Download(c.Context(), "backup-id")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(rdr, tc.IsNil)
}
