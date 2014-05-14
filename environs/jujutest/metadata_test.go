// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	"io/ioutil"
	"net/http"
	"net/url"

	gc "launchpad.net/gocheck"
)

type metadataSuite struct{}

var _ = gc.Suite(&metadataSuite{})

func (s *metadataSuite) TestCannedRoundTripper(c *gc.C) {
	aContent := "a-content"
	vrt := NewCannedRoundTripper(map[string]string{
		"a": aContent,
		"b": "b-content",
	}, nil)
	c.Assert(vrt, gc.NotNil)
	req := &http.Request{URL: &url.URL{Path: "a"}}
	resp, err := vrt.RoundTrip(req)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, gc.NotNil)
	content, err := ioutil.ReadAll(resp.Body)
	c.Assert(string(content), gc.Equals, aContent)
	c.Assert(resp.ContentLength, gc.Equals, int64(len(aContent)))
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	c.Assert(resp.Status, gc.Equals, "200 OK")
}

func (s *metadataSuite) TestCannedRoundTripperMissing(c *gc.C) {
	vrt := NewCannedRoundTripper(map[string]string{"a": "a-content"}, nil)
	c.Assert(vrt, gc.NotNil)
	req := &http.Request{URL: &url.URL{Path: "no-such-file"}}
	resp, err := vrt.RoundTrip(req)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, gc.NotNil)
	content, err := ioutil.ReadAll(resp.Body)
	c.Assert(string(content), gc.Equals, "")
	c.Assert(resp.ContentLength, gc.Equals, int64(0))
	c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)
	c.Assert(resp.Status, gc.Equals, "404 Not Found")
}
