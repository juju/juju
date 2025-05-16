// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"io"
	"net/http"
	"net/url"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

type metadataSuite struct{}

func TestMetadataSuite(t *stdtesting.T) { tc.Run(t, &metadataSuite{}) }
func (s *metadataSuite) TestCannedRoundTripper(c *tc.C) {
	aContent := "a-content"
	vrt := testing.NewCannedRoundTripper(map[string]string{
		"a": aContent,
		"b": "b-content",
	}, nil)
	c.Assert(vrt, tc.NotNil)
	req := &http.Request{URL: &url.URL{Path: "a"}}
	resp, err := vrt.RoundTrip(req)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp, tc.NotNil)
	content, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(content), tc.Equals, aContent)
	c.Assert(resp.ContentLength, tc.Equals, int64(len(aContent)))
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	c.Assert(resp.Status, tc.Equals, "200 OK")
}

func (s *metadataSuite) TestCannedRoundTripperMissing(c *tc.C) {
	vrt := testing.NewCannedRoundTripper(map[string]string{"a": "a-content"}, nil)
	c.Assert(vrt, tc.NotNil)
	req := &http.Request{URL: &url.URL{Path: "no-such-file"}}
	resp, err := vrt.RoundTrip(req)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp, tc.NotNil)
	content, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(content), tc.Equals, "")
	c.Assert(resp.ContentLength, tc.Equals, int64(0))
	c.Assert(resp.StatusCode, tc.Equals, http.StatusNotFound)
	c.Assert(resp.Status, tc.Equals, "404 Not Found")
}
