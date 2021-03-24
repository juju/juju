// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/simplestreams/testing"
)

var _ = gc.Suite(&datasourceSuite{})
var _ = gc.Suite(&datasourceHTTPSSuite{})

type datasourceSuite struct {
	testing.TestDataSuite
}

func (s *datasourceSuite) TestFetch(c *gc.C) {
	ds := testing.VerifyDefaultCloudDataSource("test", "test:")
	rc, url, err := ds.Fetch("streams/v1/tools_metadata.json")
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rc.Close() }()
	c.Assert(url, gc.Equals, "test:/streams/v1/tools_metadata.json")
	data, err := ioutil.ReadAll(rc)
	c.Assert(err, jc.ErrorIsNil)
	cloudMetadata, err := simplestreams.ParseCloudMetadata(data, "products:1.0", url, imagemetadata.ImageMetadata{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(cloudMetadata.Products), jc.GreaterThan, 0)
}

func (s *datasourceSuite) TestURL(c *gc.C) {
	ds := testing.VerifyDefaultCloudDataSource("test", "foo")
	url, err := ds.URL("bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.Equals, "foo/bar")
}

type datasourceHTTPSSuite struct {
	Server *httptest.Server
}

func (s *datasourceHTTPSSuite) SetUpTest(c *gc.C) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(resp http.ResponseWriter, req *http.Request) {
		_ = req.Body.Close()
		resp.WriteHeader(200)
		_, _ = resp.Write([]byte("Greetings!\n"))
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
	ds := testing.VerifyDefaultCloudDataSource("test", s.Server.URL)
	url, err := ds.URL("bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url, gc.Equals, s.Server.URL+"/bar")
	reader, _, err := ds.Fetch("bar")
	// The underlying failure is a x509: certificate signed by unknown authority
	// However, the urlDataSource abstraction hides that as a simple NotFound
	c.Assert(err, gc.ErrorMatches, "invalid URL \".*/bar\" not found")
	c.Check(reader, gc.IsNil)
}

func (s *datasourceHTTPSSuite) TestNonVerifyingClientSucceeds(c *gc.C) {
	ds := simplestreams.NewDataSource(simplestreams.Config{
		Description:          "test",
		BaseURL:              s.Server.URL,
		HostnameVerification: utils.NoVerifySSLHostnames,
		Priority:             simplestreams.DEFAULT_CLOUD_DATA,
	})
	url, err := ds.URL("bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url, gc.Equals, s.Server.URL+"/bar")
	reader, _, err := ds.Fetch("bar")
	// The underlying failure is a x509: certificate signed by unknown authority
	// However, the urlDataSource abstraction hides that as a simple NotFound
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = reader.Close() }()
	byteContent, err := ioutil.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(byteContent), gc.Equals, "Greetings!\n")
}
