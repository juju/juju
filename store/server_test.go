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
	"time"
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
				"sha256":   t.sha,
			}
		} else {
			expected[t.url] = map[string]interface{}{
				"revision": float64(0),
				"errors":   []interface{}{t.err},
			}
		}
		obtained := map[string]interface{}{}
		err = json.NewDecoder(rec.Body).Decode(&obtained)
		c.Assert(err, IsNil)
		c.Assert(obtained, DeepEquals, expected)
		c.Assert(rec.Header().Get("Content-Type"), Equals, "application/json")
	}

	s.checkCounterSum(c, []string{"charm-info", curl.Series, curl.Name}, false, 1)
	s.checkCounterSum(c, []string{"charm-missing", "oneiric", "non-existent"}, false, 1)
}

// checkCounterSum checks that statistics are properly collected.
// It retries a few times as they are generally collected in background.
func (s *StoreSuite) checkCounterSum(c *C, key []string, prefix bool, expected int64) {
	var sum int64
	var err error
	for retry := 0; retry < 10; retry++ {
		time.Sleep(1e8)
		sum, err = s.store.SumCounter(key, prefix)
		c.Assert(err, IsNil)
		if sum == expected {
			if expected == 0 && retry < 2 {
				continue // Wait a bit to make sure.
			}
			return
		}
	}
	c.Errorf("counter sum for %#v is %d, want %d", key, sum, expected)
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
	s.checkCounterSum(c, []string{"charm-bundle", curl.Series, curl.Name}, false, 1)
}

func (s *StoreSuite) TestDisableStats(c *C) {
	server, curl := s.prepareServer(c)

	req, err := http.NewRequest("GET", "/charm-info", nil)
	c.Assert(err, IsNil)
	req.Form = url.Values{"charms": []string{curl.String()}, "stats": []string{"0"}}
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, Equals, 200)

	req, err = http.NewRequest("GET", "/charm/"+curl.String()[3:], nil)
	c.Assert(err, IsNil)
	req.Form = url.Values{"stats": []string{"0"}}
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, Equals, 200)

	// No statistics should have been collected given the use of stats=0.
	for _, prefix := range []string{"charm-info", "charm-bundle", "charm-missing"} {
		s.checkCounterSum(c, []string{prefix}, true, 0)
	}
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
		req, err := http.NewRequest("GET", "/stats/counter/"+counter, nil)
		c.Assert(err, IsNil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		data, err := ioutil.ReadAll(rec.Body)
		c.Assert(string(data), Equals, n)

		c.Assert(rec.Header().Get("Content-Type"), Equals, "text/plain")
		c.Assert(rec.Header().Get("Content-Length"), Equals, strconv.Itoa(len(n)))
	}
}

func (s *StoreSuite) TestBlitzKey(c *C) {
	server, _ := s.prepareServer(c)

	// This is just a validation key to allow blitz.io to run
	// performance tests against the site.
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
