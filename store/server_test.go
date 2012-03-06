package store_test

import (
	"encoding/json"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/charm"
	"launchpad.net/juju/go/store"
	"net/http"
	"net/http/httptest"
	"net/url"
)

func (s *StoreSuite) TestServerCharmInfo(c *C) {
	u := charm.MustParseURL("cs:oneiric/wordpress")
	pub, err := s.store.CharmPublisher([]*charm.URL{u}, "some-digest")
	c.Assert(err, IsNil)
	err = pub.Publish(&FakeCharmDir{})
	c.Assert(err, IsNil)

	server, err := store.NewServer(s.store)
	c.Assert(err, IsNil)

	req, err := http.NewRequest("GET", "/charm-info", nil)
	c.Assert(err, IsNil)
	req.Form = url.Values{"charms": []string{u.String()}}
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	expected := map[string]interface{}{
		u.String(): map[string]interface{}{
			"revision": float64(0),
			"sha256": fakeRevZeroSha,
		},
	}
	obtained := map[string]interface{}{}
	err = json.NewDecoder(rec.Body).Decode(&obtained)
	c.Assert(err, IsNil)
	c.Assert(obtained, DeepEquals, expected)

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

func (s *StoreSuite) TestServer404(c *C) {
	server, err := store.NewServer(s.store)
	c.Assert(err, IsNil)
	for _, path := range []string{"/charm-info/foo"} {
		req, err := http.NewRequest("GET", path, nil)
		c.Assert(err, IsNil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		c.Assert(rec.Code, Equals, 404)
	}
}
