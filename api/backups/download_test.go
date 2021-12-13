// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"
)

type downloadSuite struct {
	baseSuite
}

var _ = gc.Suite(&downloadSuite{})

func (s *downloadSuite) TestDownload(c *gc.C) {
	defer s.setupMocks(c).Finish()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.URL.String(), gc.Equals, "/backups")
		_, err := w.Write([]byte("success"))
		c.Assert(err, jc.ErrorIsNil)
	}))
	defer srv.Close()
	httpClient := &httprequest.Client{BaseURL: srv.URL}

	s.apiCaller.EXPECT().HTTPClient().Return(httpClient, nil)
	s.apiCaller.EXPECT().Context().Return(context.TODO())

	client := s.newClient()
	rdr, err := client.Download("/path/to/backup")
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rdr.Close() }()

	data, err := ioutil.ReadAll(rdr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "success")
}
