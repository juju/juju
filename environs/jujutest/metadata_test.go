// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"net/url"
)

type metadataSuite struct{}

var _ = Suite(&metadataSuite{})

func (s *metadataSuite) TestCannedRoundTripper(c *C) {
	aContent := "a-content"
	vrt := NewCannedRoundTripper(map[string]string{
		"a": aContent,
		"b": "b-content",
	}, nil)
	c.Assert(vrt, NotNil)
	req := &http.Request{URL: &url.URL{Path: "a"}}
	resp, err := vrt.RoundTrip(req)
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	content, err := ioutil.ReadAll(resp.Body)
	c.Assert(string(content), Equals, aContent)
	c.Assert(resp.ContentLength, Equals, int64(len(aContent)))
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	c.Assert(resp.Status, Equals, "200 OK")
}

func (s *metadataSuite) TestCannedRoundTripperMissing(c *C) {
	vrt := NewCannedRoundTripper(map[string]string{"a": "a-content"}, nil)
	c.Assert(vrt, NotNil)
	req := &http.Request{URL: &url.URL{Path: "no-such-file"}}
	resp, err := vrt.RoundTrip(req)
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	content, err := ioutil.ReadAll(resp.Body)
	c.Assert(string(content), Equals, "")
	c.Assert(resp.ContentLength, Equals, int64(0))
	c.Assert(resp.StatusCode, Equals, http.StatusNotFound)
	c.Assert(resp.Status, Equals, "404 Not Found")
}
