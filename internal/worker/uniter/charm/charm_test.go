// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	jujucharm "github.com/juju/juju/internal/charm"
	charmtesting "github.com/juju/juju/internal/charm/testing"
	"github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/testcharms"
)

// bundleReader is a charm.BundleReader that lets us mock out the bundles we
// deploy to test the Deployers.
type bundleReader struct {
	bundles     map[string]charm.Bundle
	stopWaiting <-chan struct{}
}

// EnableWaitForAbort allows us to test that a Deployer.Stage call passes its abort
// chan down to its BundleReader's Read method. If you call EnableWaitForAbort, the
// next call to Read will block until either the abort chan is closed (in which case
// it will return an error) or the stopWaiting chan is closed (in which case it
// will return the bundle).
func (br *bundleReader) EnableWaitForAbort() (stopWaiting chan struct{}) {
	stopWaiting = make(chan struct{})
	br.stopWaiting = stopWaiting
	return stopWaiting
}

// Read implements the BundleReader interface.
func (br *bundleReader) Read(ctx context.Context, info charm.BundleInfo) (charm.Bundle, error) {
	bundle, ok := br.bundles[info.URL()]
	if !ok {
		return nil, fmt.Errorf("no such charm!")
	}
	if br.stopWaiting != nil {
		// EnableWaitForAbort is a one-time wait; make sure we clear it.
		defer func() { br.stopWaiting = nil }()
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("charm read aborted")
		case <-br.stopWaiting:
			// We can stop waiting for the abort chan and return the bundle.
		}
	}
	return bundle, nil
}

func (br *bundleReader) AddCustomBundle(c *tc.C, url *jujucharm.URL, customize func(path string)) charm.BundleInfo {
	base := c.MkDir()
	dirpath := testcharms.Repo.ClonedDirPath(base, "dummy")
	if customize != nil {
		customize(dirpath)
	}
	dir, err := charmtesting.ReadCharmDir(dirpath)
	c.Assert(err, tc.ErrorIsNil)
	bunpath := filepath.Join(base, "bundle")
	err = dir.ArchiveToPath(bunpath)
	c.Assert(err, tc.ErrorIsNil)
	bundle, err := jujucharm.ReadCharmArchive(bunpath)
	c.Assert(err, tc.ErrorIsNil)
	return br.AddBundle(url, bundle)
}

func (br *bundleReader) AddBundle(url *jujucharm.URL, bundle charm.Bundle) charm.BundleInfo {
	if br.bundles == nil {
		br.bundles = map[string]charm.Bundle{}
	}
	br.bundles[url.String()] = bundle
	return &bundleInfo{nil, url.String()}
}

type bundleInfo struct {
	charm.BundleInfo
	url string
}

func (info *bundleInfo) URL() string {
	return info.url
}

type mockBundle struct {
	paths  set.Strings
	expand func(dir string) error
}

func (b mockBundle) ArchiveMembers() (set.Strings, error) {
	// TODO(dfc) this looks like set.Strings().Duplicate()
	return set.NewStrings(b.paths.Values()...), nil
}

func (b mockBundle) ExpandTo(dir string) error {
	if b.expand != nil {
		return b.expand(dir)
	}
	return nil
}

func charmURL(revision int) *jujucharm.URL {
	baseURL := jujucharm.MustParseURL("ch:s/c")
	return baseURL.WithRevision(revision)
}
