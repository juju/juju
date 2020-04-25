// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"net/url"

	"github.com/juju/charm/v7"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"
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
	m, err := macaroon.New([]byte("key"), []byte("id"), "loc", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	ms := macaroon.Slice{m}
	httpbakery.SetCookie(jar, u, MacaroonNamespace, ms)
	// c.Assert(cache[ch], gc.DeepEquals, ms)
	MacaroonEquals(c, cache[ch][0], ms[0])
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
	m, err := macaroon.New([]byte("key"), []byte("id"), "loc", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	ms := macaroon.Slice{m}
	err = jar.Deactivate()
	c.Assert(err, jc.ErrorIsNil)
	httpbakery.SetCookie(jar, u, MacaroonNamespace, ms)
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

func MacaroonEquals(c *gc.C, m1, m2 *macaroon.Macaroon) {
	c.Assert(m1.Id(), jc.DeepEquals, m2.Id())
	c.Assert(m1.Signature(), jc.DeepEquals, m2.Signature())
	c.Assert(m1.Location(), jc.DeepEquals, m2.Location())
}
