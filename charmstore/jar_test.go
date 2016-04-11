package charmstore

import (
	"net/url"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"
)

var _ = gc.Suite(&MacaroonJarSuite{})

type MacaroonJarSuite struct {
	testing.IsolationSuite
}

func (MacaroonJarSuite) TestActivate(c *gc.C) {
	cache := fakeCache{}
	u, err := url.Parse("http://charmstore.com")
	c.Assert(err, jc.ErrorIsNil)
	jar, err := newMacaroonJar(cache, u)
	c.Assert(err, jc.ErrorIsNil)
	ch := charm.MustParseURL("cs:mysql")
	err = jar.Activate(ch)
	c.Assert(err, jc.ErrorIsNil)
	m, err := macaroon.New([]byte("key"), "id", "loc")
	c.Assert(err, jc.ErrorIsNil)
	ms := macaroon.Slice{m}
	httpbakery.SetCookie(jar, u, ms)
	c.Assert(cache[ch], gc.DeepEquals, ms)
}

func (MacaroonJarSuite) TestDeactivate(c *gc.C) {
	cache := fakeCache{}
	u, err := url.Parse("http://charmstore.com")
	c.Assert(err, jc.ErrorIsNil)
	jar, err := newMacaroonJar(cache, u)
	c.Assert(err, jc.ErrorIsNil)
	ch := charm.MustParseURL("cs:mysql")
	err = jar.Activate(ch)
	c.Assert(err, jc.ErrorIsNil)
	m, err := macaroon.New([]byte("key"), "id", "loc")
	c.Assert(err, jc.ErrorIsNil)
	ms := macaroon.Slice{m}
	err = jar.Deactivate()
	c.Assert(err, jc.ErrorIsNil)
	httpbakery.SetCookie(jar, u, ms)
	c.Assert(cache, gc.HasLen, 0)
	c.Assert(jar.Cookies(u), gc.HasLen, 1)
}

type fakeCache map[*charm.URL]macaroon.Slice

func (f fakeCache) Set(u *charm.URL, m macaroon.Slice) error {
	f[u] = m
	return nil
}

func (f fakeCache) Get(u *charm.URL) (macaroon.Slice, error) {
	return f[u], nil
}
