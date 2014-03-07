// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/simplestreams/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

var _ = gc.Suite(&datasourceSuite{})
var _ = gc.Suite(&datasourceHTTPSSuite{})

type datasourceSuite struct {
	testing.TestDataSuite
}

func (s *datasourceSuite) TestFetch(c *gc.C) {
	ds := simplestreams.NewURLDataSource("test", "test:", simplestreams.VerifySSLHostnames)
	rc, url, err := ds.Fetch("streams/v1/tools_metadata.json")
	c.Assert(err, gc.IsNil)
	defer rc.Close()
	c.Assert(url, gc.Equals, "test:/streams/v1/tools_metadata.json")
	data, err := ioutil.ReadAll(rc)
	cloudMetadata, err := simplestreams.ParseCloudMetadata(data, "products:1.0", url, imagemetadata.ImageMetadata{})
	c.Assert(err, gc.IsNil)
	c.Assert(len(cloudMetadata.Products), jc.GreaterThan, 0)
}

func (s *datasourceSuite) TestURL(c *gc.C) {
	ds := simplestreams.NewURLDataSource("test", "foo", simplestreams.VerifySSLHostnames)
	url, err := ds.URL("bar")
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.Equals, "foo/bar")
}

type datasourceHTTPSSuite struct {
	Server *httptest.Server
}

func (s *datasourceHTTPSSuite) SetUpTest(c *gc.C) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(resp http.ResponseWriter, req *http.Request) {
		req.Body.Close()
		resp.WriteHeader(200)
		resp.Write([]byte("Greetings!\n"))
	})
	s.Server = httptest.NewTLSServer(mux)
}

func (s *datasourceHTTPSSuite) TearDownTest(c *gc.C) {
	if s.Server != nil {
		s.Server.Close()
		s.Server = nil
	}
}

func (s *datasourceHTTPSSuite) TestNormalClientFails(c *gc.C) {
	ds := simplestreams.NewURLDataSource("test", s.Server.URL, simplestreams.VerifySSLHostnames)
	url, err := ds.URL("bar")
	c.Assert(err, gc.IsNil)
	c.Check(url, gc.Equals, s.Server.URL+"/bar")
	reader, _, err := ds.Fetch("bar")
	// The underlying failure is a x509: certificate signed by unknown authority
	// However, the urlDataSource abstraction hides that as a simple NotFound
	c.Assert(err, gc.ErrorMatches, "invalid URL \".*/bar\" not found")
	c.Check(reader, gc.IsNil)
}

func (s *datasourceHTTPSSuite) TestNonVerifyingClientSucceeds(c *gc.C) {
	ds := simplestreams.NewURLDataSource("test", s.Server.URL, simplestreams.NoVerifySSLHostnames)
	url, err := ds.URL("bar")
	c.Assert(err, gc.IsNil)
	c.Check(url, gc.Equals, s.Server.URL+"/bar")
	reader, _, err := ds.Fetch("bar")
	// The underlying failure is a x509: certificate signed by unknown authority
	// However, the urlDataSource abstraction hides that as a simple NotFound
	c.Assert(err, gc.IsNil)
	defer reader.Close()
	byteContent, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(string(byteContent), gc.Equals, "Greetings!\n")
}
