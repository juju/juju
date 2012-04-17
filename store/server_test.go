package store_test

import (
	"encoding/json"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/charm"
	"launchpad.net/juju/go/store"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
)

func (s *StoreSuite) prepareServer(c *C) (*store.Server, *charm.URL) {
	curl := charm.MustParseURL("cs:oneiric/wordpress")
	pub, err := s.store.CharmPublisher([]*charm.URL{curl}, "some-digest")
	c.Assert(err, IsNil)
	err = pub.Publish(&FakeCharmDir{})
	c.Assert(err, IsNil)

	server, err := store.NewServer(s.store)
	c.Assert(err, IsNil)
	return server, curl
}

func (s *StoreSuite) TestServerCharmInfo(c *C) {
	server, curl := s.prepareServer(c)

	req, err := http.NewRequest("GET", "/charm-info", nil)
	c.Assert(err, IsNil)
	req.Form = url.Values{"charms": []string{curl.String()}}
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	expected := map[string]interface{}{
		curl.String(): map[string]interface{}{
			"revision": float64(0),
			"sha256":   fakeRevZeroSha,
		},
	}
	obtained := map[string]interface{}{}
	err = json.NewDecoder(rec.Body).Decode(&obtained)
	c.Assert(err, IsNil)
	c.Assert(obtained, DeepEquals, expected)
	c.Assert(rec.Header().Get("Content-Type"), Equals, "application/json")

	// Now check an error condition.
	req.Form["charms"] = []string{"cs:bad"}
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	expected = map[string]interface{}{
		"cs:bad": map[string]interface{}{
			"revision": float64(0),
			"errors":   []interface{}{`charm URL without series: "cs:bad"`},
		},
	}
	obtained = map[string]interface{}{}
	err = json.NewDecoder(rec.Body).Decode(&obtained)
	c.Assert(err, IsNil)
	c.Assert(obtained, DeepEquals, expected)
}

func (s *StoreSuite) TestCharmStreaming(c *C) {
	server, curl := s.prepareServer(c)

	req, err := http.NewRequest("GET", "/charm/"+curl.String()[3:], nil)
	c.Assert(err, IsNil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	data, err := ioutil.ReadAll(rec.Body)
	c.Assert(string(data), Equals, "charm-revision-0")

	c.Assert(rec.Header().Get("Connection"), Equals, "close")
	c.Assert(rec.Header().Get("Content-Type"), Equals, "application/octet-stream")
	c.Assert(rec.Header().Get("Content-Length"), Equals, "16")
}

// This is necessary to run performance tests with blitz.io.
func (s *StoreSuite) TestBlitzKey(c *C) {
	server, _ := s.prepareServer(c)

	req, err := http.NewRequest("GET", "/mu-35700a31-6bf320ca-a800b670-05f845ee", nil)
	c.Assert(err, IsNil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	data, err := ioutil.ReadAll(rec.Body)
	c.Assert(string(data), Equals, "42")

	c.Assert(rec.Header().Get("Connection"), Equals, "close")
	c.Assert(rec.Header().Get("Content-Type"), Equals, "text/plain")
	c.Assert(rec.Header().Get("Content-Length"), Equals, "2")
}

func (s *StoreSuite) TestServerStatus(c *C) {
	server, err := store.NewServer(s.store)
	c.Assert(err, IsNil)
	tests := []struct {
		path string
		code int
	}{
		{"/charm-info/any", 404},
		{"/charm/bad-url", 404},
		{"/charm/bad-series/wordpress", 404},
		{"/stats/counter/", 403},
		{"/stats/counter/*", 403},
		{"/stats/counter/any/", 404},
		{"/stats/", 404},
		{"/stats/any", 404},
	}
	for _, test := range tests {
		req, err := http.NewRequest("GET", test.path, nil)
		c.Assert(err, IsNil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		c.Assert(rec.Code, Equals, test.code, Commentf("Path: %s", test.path))
	}
}

func (s *StoreSuite) TestRootRedirect(c *C) {
	server, err := store.NewServer(s.store)
	c.Assert(err, IsNil)
	req, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, Equals, 303)
	c.Assert(rec.Header().Get("Location"), Equals, "https://juju.ubuntu.com")
}

func (s *StoreSuite) TestStatsCounter(c *C) {
	for _, key := range [][]string{{"a", "b"}, {"a", "b"}, {"a"}} {
		err := s.store.IncCounter(key)
		c.Assert(err, IsNil)
	}

	server, _ := s.prepareServer(c)

	expected := map[string]string{
		"a:b": "2",
		"a:*": "3",
		"a":   "1",
	}

	for counter, n := range expected {
		req, err := http.NewRequest("GET", "/stats/counter/" + counter, nil)
		c.Assert(err, IsNil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		data, err := ioutil.ReadAll(rec.Body)
		c.Assert(string(data), Equals, n)

		c.Assert(rec.Header().Get("Content-Type"), Equals, "text/plain")
		c.Assert(rec.Header().Get("Content-Length"), Equals, strconv.Itoa(len(n)))
	}
}
