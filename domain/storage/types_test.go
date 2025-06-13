// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/storage"
	internalstorage "github.com/juju/juju/internal/storage"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	"github.com/juju/juju/internal/testhelpers"
)

type typesSuite struct {
	testhelpers.IsolationSuite
}

func TestTypesSuite(t *testing.T) {
	tc.Run(t, &typesSuite{})
}

func (s *typesSuite) TestDefaultStoragePools(c *tc.C) {
	p1, err := internalstorage.NewConfig("pool1", "whatever", map[string]interface{}{"1": "2"})
	c.Assert(err, tc.ErrorIsNil)
	p2, err := internalstorage.NewConfig("pool2", "whatever", map[string]interface{}{"3": "4"})
	c.Assert(err, tc.ErrorIsNil)
	provider := &dummystorage.StorageProvider{
		DefaultPools_: []*internalstorage.Config{p1, p2},
	}

	registry := internalstorage.StaticProviderRegistry{
		Providers: map[internalstorage.ProviderType]internalstorage.Provider{
			"whatever": provider,
		},
	}

	pools, err := storage.DefaultStoragePools(registry)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pools, tc.SameContents, []*internalstorage.Config{p1, p2})
}

func (s *typesSuite) TestNamesValues(c *tc.C) {
	n := storage.Names{"a", "b", "c", "a", ""}
	c.Assert(n.Values(), tc.SameContents, []string{"a", "b", "c"})
}

func (s *typesSuite) TestNamesContains(c *tc.C) {
	n := storage.Names{"a", "b", "c"}
	c.Assert(n.Contains("a"), tc.Equals, true)
	c.Assert(n.Contains("b"), tc.Equals, true)
	c.Assert(n.Contains("c"), tc.Equals, true)
	c.Assert(n.Contains("d"), tc.Equals, false)
	c.Assert(n.Contains(""), tc.Equals, false)
}

func (s *typesSuite) TestProvidersValues(c *tc.C) {
	p := storage.Providers{"x", "y", "z", "x", ""}
	c.Assert(p.Values(), tc.SameContents, []string{"x", "y", "z"})
}

func (s *typesSuite) TestProvidersContains(c *tc.C) {
	p := storage.Providers{"x", "y", "z"}
	c.Assert(p.Contains("x"), tc.Equals, true)
	c.Assert(p.Contains("y"), tc.Equals, true)
	c.Assert(p.Contains("z"), tc.Equals, true)
	c.Assert(p.Contains("a"), tc.Equals, false)
	c.Assert(p.Contains(""), tc.Equals, false)
}
