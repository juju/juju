package jujutest

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"net/url"
)

type metadataSuite struct{}

var _ = Suite(&metadataSuite{})

func (s *metadataSuite) TestVirtualRoundTripper(c *C) {
	a_content := "a-content"
	vrt := NewVirtualRoundTripper([]FileContent{
		{"a", a_content},
		{"b", "b-content"},
	})
	c.Assert(vrt, NotNil)
	req := &http.Request{URL: &url.URL{Path: "a"}}
	resp, err := vrt.RoundTrip(req)
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	content, err := ioutil.ReadAll(resp.Body)
	c.Assert(string(content), Equals, a_content)
	c.Assert(resp.ContentLength, Equals, int64(len(a_content)))
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	c.Assert(resp.Status, Equals, "200 OK")
}

func (s *metadataSuite) TestVirtualRoundTripperMissing(c *C) {
	vrt := NewVirtualRoundTripper([]FileContent{
		{"a", "a-content"},
	})
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
