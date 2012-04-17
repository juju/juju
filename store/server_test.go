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

	var tests = []struct{ url, sha, err string }{
		{curl.String(), fakeRevZeroSha, ""},
		{"cs:oneiric/non-existent", "", "entry not found"},
		{"cs:bad", "", `charm URL without series: "cs:bad"`},
	}

	for _, t := range tests {
		req.Form = url.Values{"charms": []string{t.url}}
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		expected := make(map[string]interface{})
		if t.sha != "" {
			expected[t.url] = map[string]interface{}{
				"revision": float64(0),
				"sha256": t.sha,
			}
		} else {
			expected[t.url] = map[string]interface{}{
				"revision": float64(0),
				"errors": []interface{}{t.err},
			}
		}
		obtained := map[string]interface{}{}
		err = json.NewDecoder(rec.Body).Decode(&obtained)
		c.Assert(err, IsNil)
		c.Assert(obtained, DeepEquals, expected)
		c.Assert(rec.Header().Get("Content-Type"), Equals, "application/json")
	}

	// Check that statistics were properly collected.
	sum, err := s.store.SumCounter([]string{"charm-info", curl.Series, curl.Name}, false)
	c.Assert(err, IsNil)
	c.Assert(sum, Equals, int64(1))

	sum, err = s.store.SumCounter([]string{"charm-missing", "oneiric", "non-existent"}, false)
	c.Assert(err, IsNil)
	c.Assert(sum, Equals, int64(1))
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

	// Check that it was accounted for in statistics.
	sum, err := s.store.SumCounter([]string{"charm-bundle", curl.Series, curl.Name}, false)
	c.Assert(err, IsNil)
	c.Assert(sum, Equals, int64(1))
}

func (s *StoreSuite) TestDisableStats(c *C) {
	server, curl := s.prepareServer(c)

	req, err := http.NewRequest("GET", "/charm-info", nil)
	c.Assert(err, IsNil)
	req.Form = url.Values{"charms": []string{curl.String()}, "stats": []string{"0"}}
	server.ServeHTTP(httptest.NewRecorder(), req)

	req, err = http.NewRequest("GET", "/charms/"+curl.String()[:3], nil)
	c.Assert(err, IsNil)
	req.Form = url.Values{"stats": []string{"0"}}
	server.ServeHTTP(httptest.NewRecorder(), req)

	// No statistics should have been collected given the use of stats=0.
	for _, prefix := range []string{"charm-info", "charm-bundle", "charm-missing"} {
		sum, err := s.store.SumCounter([]string{prefix}, true)
		c.Assert(err, IsNil)
		c.Assert(sum, Equals, int64(0), Commentf("prefix: %s", prefix))
	}
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
