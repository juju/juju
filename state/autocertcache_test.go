// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	statetesting "github.com/juju/juju/state/testing"
)

type autocertCacheSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&autocertCacheSuite{})

func (s *autocertCacheSuite) TestCachePutGet(c *gc.C) {
	ctx := context.Background()
	cache := s.State.AutocertCache()

	err := cache.Put(ctx, "a", []byte("aval"))
	c.Assert(err, jc.ErrorIsNil)
	err = cache.Put(ctx, "b", []byte("bval"))
	c.Assert(err, jc.ErrorIsNil)

	// Check that we can get the existing entries.
	data, err := cache.Get(ctx, "a")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "aval")

	data, err = cache.Get(ctx, "b")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "bval")
}

func (s *autocertCacheSuite) TestGetNonexistentEntry(c *gc.C) {
	ctx := context.Background()
	cache := s.State.AutocertCache()

	// Getting a non-existent entry must return ErrCacheMiss.
	data, err := cache.Get(ctx, "c")
	c.Assert(err, gc.Equals, autocert.ErrCacheMiss)
	c.Assert(data, gc.IsNil)
}

func (s *autocertCacheSuite) TestDelete(c *gc.C) {
	ctx := context.Background()
	cache := s.State.AutocertCache()

	err := cache.Put(ctx, "a", []byte("aval"))
	c.Assert(err, jc.ErrorIsNil)
	err = cache.Put(ctx, "b", []byte("bval"))
	c.Assert(err, jc.ErrorIsNil)

	// Check that we can delete an entry.
	err = cache.Delete(ctx, "b")
	c.Assert(err, jc.ErrorIsNil)

	data, err := cache.Get(ctx, "b")
	c.Assert(err, gc.Equals, autocert.ErrCacheMiss)
	c.Assert(data, gc.IsNil)

	// Check that the non-deleted entry is still there.
	data, err = cache.Get(ctx, "a")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "aval")
}

func (s *autocertCacheSuite) TestDeleteNonexistentEntry(c *gc.C) {
	ctx := context.Background()
	cache := s.State.AutocertCache()

	err := cache.Delete(ctx, "a")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *autocertCacheSuite) TestPutExistingEntry(c *gc.C) {
	ctx := context.Background()
	cache := s.State.AutocertCache()

	err := cache.Put(ctx, "a", []byte("aval"))
	c.Assert(err, jc.ErrorIsNil)

	err = cache.Put(ctx, "a", []byte("aval2"))
	c.Assert(err, jc.ErrorIsNil)

	data, err := cache.Get(ctx, "a")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "aval2")
}
