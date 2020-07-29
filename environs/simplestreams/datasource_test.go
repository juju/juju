// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/simplestreams/testing"
)

var _ = gc.Suite(&datasourceSuite{})
var _ = gc.Suite(&datasourceHTTPSSuite{})

type datasourceSuite struct {
}

func (s *datasourceSuite) TestFetch(c *gc.C) {
	server := httptest.NewServer(&testDataHandler{})
	defer server.Close()
	ds := testing.VerifyDefaultCloudDataSource("test", server.URL)
	rc, url, err := ds.Fetch("streams/v1/tools_metadata.json")
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rc.Close() }()
	c.Assert(url, gc.Equals, fmt.Sprintf("%s/streams/v1/tools_metadata.json", server.URL))
	data, err := ioutil.ReadAll(rc)
	c.Assert(err, jc.ErrorIsNil)
	cloudMetadata, err := simplestreams.ParseCloudMetadata(data, testing.Product_v1, url, imagemetadata.ImageMetadata{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(cloudMetadata.Products), jc.GreaterThan, 0)
}

func (s *datasourceSuite) TestURL(c *gc.C) {
	ds := testing.VerifyDefaultCloudDataSource("test", "foo")
	url, err := ds.URL("bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.Equals, "foo/bar")
}

type testDataHandler struct{}

func (h *testDataHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/unauth":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		return
	case "/streams/v1/tools_metadata.json":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, unsignedProduct)
		return
	default:
		http.Error(w, r.URL.Path, 404)
		return
	}
}

var unsignedProduct = `
{
 "updated": "Wed, 01 May 2013 13:31:26 +0000",
 "content_id": "com.ubuntu.cloud:released:aws",
 "datatype": "content-download",
 "products": {
   "com.ubuntu.juju:12.04:amd64": {
    "arch": "amd64",
    "release": "precise",
    "versions": {
     "20130806": {
      "items": {
       "1130preciseamd64": {
        "version": "1.13.0",
        "size": 2973595,
        "path": "tools/releases/20130806/juju-1.13.0-precise-amd64.tgz",
        "ftype": "tar.gz",
        "sha256": "447aeb6a934a5eaec4f703eda4ef2dde"
       }
      }
     }
    }
   }
 },
 "format": "products:1.0"
}
`

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
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
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
