// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"os"
	"path/filepath"

	jujucharm "github.com/juju/charm/v8"

	"github.com/juju/juju/worker/uniter/charm"
)

// mockDeployer implements Deployer.
type mockDeployer struct {
	charmPath string
	dataPath  string
	bundles   charm.BundleReader

	bundle   charm.Bundle
	staged   *jujucharm.URL
	curl     *jujucharm.URL
	deployed bool
	err      error
}

func (m *mockDeployer) Stage(info charm.BundleInfo, abort <-chan struct{}) error {
	m.staged = info.URL()
	var err error
	m.bundle, err = m.bundles.Read(info, abort)
	return err
}

func (m *mockDeployer) Deploy() error {
	if err := os.MkdirAll(m.charmPath, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(m.dataPath, "manifests"), 0755); err != nil {
		return err
	}
	if m.err != nil {
		return m.err
	}
	if err := m.bundle.ExpandTo(m.charmPath); err != nil {
		return err
	}
	m.deployed = true
	m.curl = m.staged
	return nil
}
