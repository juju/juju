// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"fmt"
	"os"
	"path/filepath"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	corecharm "launchpad.net/juju-core/charm"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/worker/uniter/charm"
)

func TestPackage(t *stdtesting.T) {
	// TODO(fwereade) 2014-03-21 not-worth-a-bug-number
	// rewrite BundlesDir tests to use the mocks below and not require an API
	// server and associated gubbins.
	coretesting.MgoTestPackage(t)
}

// bundleReader is a charm.BundleReader that lets us mock out the bundles we
// deploy to test the Deployers.
type bundleReader struct {
	bundles   map[string]charm.Bundle
	waitAbort <-chan struct{}
}

// Read implements the BundleReader interface.
func (br *bundleReader) Read(info charm.BundleInfo, abort <-chan struct{}) (charm.Bundle, error) {
	bundle, ok := br.bundles[info.URL().String()]
	if !ok {
		return nil, fmt.Errorf("no such charm!")
	}
	if br.waitAbort == nil {
		close(br.SetAbortWait())
	}
	defer func() { br.waitAbort = nil }()
	select {
	case <-abort:
		return nil, fmt.Errorf("charm read aborted")
	case <-br.waitAbort:
	}
	return bundle, nil
}

func (br *bundleReader) SetAbortWait() chan struct{} {
	waitAbort := make(chan struct{})
	br.waitAbort = waitAbort
	return waitAbort
}

func (br *bundleReader) AddCustomBundle(c *gc.C, url *corecharm.URL, customize func(path string)) charm.BundleInfo {
	base := c.MkDir()
	dirpath := coretesting.Charms.ClonedDirPath(base, "dummy")
	customize(dirpath)
	dir, err := corecharm.ReadDir(dirpath)
	c.Assert(err, gc.IsNil)
	err = dir.SetDiskRevision(url.Revision)
	c.Assert(err, gc.IsNil)
	bunpath := filepath.Join(base, "bundle")
	file, err := os.Create(bunpath)
	c.Assert(err, gc.IsNil)
	defer file.Close()
	err = dir.BundleTo(file)
	c.Assert(err, gc.IsNil)
	bundle, err := corecharm.ReadBundle(bunpath)
	c.Assert(err, gc.IsNil)
	return br.AddBundle(c, url, bundle)
}

func (br *bundleReader) AddBundle(c *gc.C, url *corecharm.URL, bundle charm.Bundle) charm.BundleInfo {
	if br.bundles == nil {
		br.bundles = map[string]charm.Bundle{}
	}
	br.bundles[url.String()] = bundle
	return &bundleInfo{nil, url}
}

type bundleInfo struct {
	charm.BundleInfo
	url *corecharm.URL
}

func (info *bundleInfo) URL() *corecharm.URL {
	return info.url
}

type mockBundle struct {
	paths  set.Strings
	expand func(dir string) error
}

func (b mockBundle) Manifest() (set.Strings, error) {
	return set.NewStrings(b.paths.Values()...), nil
}

func (b mockBundle) ExpandTo(dir string) error {
	return b.expand(dir)
}
