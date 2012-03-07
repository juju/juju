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
			"sha256": fakeRevZeroSha,
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
			"errors": []interface{}{`charm URL without series: "cs:bad"`},
		},
	}
	obtained = map[string]interface{}{}
	err = json.NewDecoder(rec.Body).Decode(&obtained)
	c.Assert(err, IsNil)
	c.Assert(obtained, DeepEquals, expected)
}

func (s *StoreSuite) TestCharmStreaming(c *C) {
	server, curl := s.prepareServer(c)

	req, err := http.NewRequest("GET", "/charm/" + curl.String()[3:], nil)
	c.Assert(err, IsNil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	data, err := ioutil.ReadAll(rec.Body)
	c.Assert(string(data), Equals, "charm-revision-0")

	c.Assert(rec.Header().Get("Content-Type"), Equals, "application/octet-stream")
}

func (s *StoreSuite) TestServer404(c *C) {
	server, err := store.NewServer(s.store)
	c.Assert(err, IsNil)
	tests := []string{
		"/charm-info/any",
		"/charm/bad-url",
		"/charm/bad-series/wordpress",
	}
	for _, path := range tests {
		req, err := http.NewRequest("GET", path, nil)
		c.Assert(err, IsNil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		c.Assert(rec.Code, Equals, 404)
	}
}
