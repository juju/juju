// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"net/http"
	"net/http/httptest"
	stdtesting "testing"

	"github.com/juju/tc"
	"gopkg.in/httprequest.v1"
)

type downloadSuite struct {
	baseSuite
}

func TestDownloadSuite(t *stdtesting.T) {
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

	s.apiCaller.EXPECT().HTTPClient().Return(httpClient, nil)

	client := s.newClient()
	rdr, err := client.Download(c.Context(), "/path/to/backup")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rdr.Close() }()

	data, err := io.ReadAll(rdr)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, "success")
}
