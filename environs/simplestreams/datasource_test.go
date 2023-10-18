// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams_test

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/simplestreams/testing"
)

var _ = gc.Suite(&datasourceSuite{})
var _ = gc.Suite(&datasourceHTTPSSuite{})

type datasourceSuite struct {
}

func (s *datasourceSuite) assertFetch(c *gc.C, compressed bool) {
	server := httptest.NewServer(&testDataHandler{supportsGzip: compressed})
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

func (s *datasourceSuite) TestFetch(c *gc.C) {
	s.assertFetch(c, false)
}

func (s *datasourceSuite) TestFetchGzip(c *gc.C) {
	s.assertFetch(c, true)
}

func (s *datasourceSuite) TestURL(c *gc.C) {
	ds := testing.VerifyDefaultCloudDataSource("test", "foo")
	url, err := ds.URL("bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.Equals, "foo/bar")
}

func (s *datasourceSuite) TestRetry(c *gc.C) {
	handler := &testDataHandler{}
	server := httptest.NewServer(handler)
	defer server.Close()
	ds := simplestreams.NewDataSource(simplestreams.Config{
		Description: "test",
		BaseURL:     server.URL,
		Priority:    simplestreams.DEFAULT_CLOUD_DATA,
		Clock:       testclock.NewDilatedWallClock(10 * time.Millisecond),
	})
	_, _, err := ds.Fetch("500")
	c.Assert(err, gc.NotNil)
	c.Assert(handler.numReq.Load(), gc.Equals, int64(3))
}

type testDataHandler struct {
	supportsGzip bool
	numReq       atomic.Int64
}

func (h *testDataHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.numReq.Add(1)
	var out io.Writer = w
	switch r.URL.Path {
	case "/unauth":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		return
	case "/streams/v1/tools_metadata.json":
		w.Header().Set("Content-Type", "application/json")
		// So long as the underlying http transport has not had DisableCompression
		// set to false, the gzip request and decompression is handled transparently.
		// This tests that we haven't accidentally turned off compression for the
		// default http client used by Juju.
		if h.supportsGzip {
			if r.Header.Get("Accept-Encoding") != "gzip" {
				http.Error(w, "Accept-Encoding header missing", 400)
				return
			}
			w.Header().Set("Content-Encoding", "gzip")
			gout := gzip.NewWriter(w)
			defer func() { _ = gout.Close() }()
			_, _ = gout.Write([]byte(unsignedProduct))

		} else {
			_, _ = io.WriteString(out, unsignedProduct)
		}
		w.WriteHeader(http.StatusOK)
		return
	case "/500":
		http.Error(w, r.URL.Path, 500)
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
	server *httptest.Server
	clock  testclock.AdvanceableClock
}

func (s *datasourceHTTPSSuite) SetUpTest(c *gc.C) {
	s.clock = testclock.NewDilatedWallClock(10 * time.Millisecond)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(resp http.ResponseWriter, req *http.Request) {
		_ = req.Body.Close()
		resp.WriteHeader(200)
		_, _ = resp.Write([]byte("Greetings!\n"))
	})
	s.server = httptest.NewTLSServer(mux)
}

func (s *datasourceHTTPSSuite) TearDownTest(c *gc.C) {
	if s.server != nil {
		s.server.Close()
		s.server = nil
	}
}

func (s *datasourceHTTPSSuite) TestNormalClientFails(c *gc.C) {
	ds := testing.VerifyDefaultCloudDataSource("test", s.server.URL)
	url, err := ds.URL("bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url, gc.Equals, s.server.URL+"/bar")
	reader, _, err := ds.Fetch("bar")
	c.Assert(err, gc.ErrorMatches, `.*x509: certificate signed by unknown authority`)
	c.Check(reader, gc.IsNil)
}

func (s *datasourceHTTPSSuite) TestNonVerifyingClientSucceeds(c *gc.C) {
	ds := simplestreams.NewDataSource(simplestreams.Config{
		Description:          "test",
		BaseURL:              s.server.URL,
		HostnameVerification: false,
		Priority:             simplestreams.DEFAULT_CLOUD_DATA,
		Clock:                s.clock,
	})
	url, err := ds.URL("bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url, gc.Equals, s.server.URL+"/bar")
	reader, _, err := ds.Fetch("bar")
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = reader.Close() }()
	byteContent, err := ioutil.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(byteContent), gc.Equals, "Greetings!\n")
}

func (s *datasourceHTTPSSuite) TestClientTransportCompression(c *gc.C) {
	ds := simplestreams.NewDataSource(simplestreams.Config{
		Description:          "test",
		BaseURL:              s.server.URL,
		HostnameVerification: false,
		Priority:             simplestreams.DEFAULT_CLOUD_DATA,
		Clock:                s.clock,
	})
	httpClient := simplestreams.HttpClient(ds)
	c.Assert(httpClient, gc.NotNil)
	tr, ok := httpClient.HTTPClient.(*http.Client).Transport.(*http.Transport)
	c.Assert(ok, jc.IsTrue)
	c.Assert(tr.DisableCompression, jc.IsFalse)
}
