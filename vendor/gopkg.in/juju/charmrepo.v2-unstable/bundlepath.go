// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrepo // import "gopkg.in/juju/charmrepo.v2-unstable"

import (
	"os"
	"path/filepath"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
)

// NewBundleAtPath creates and returns a bundle at a given path,
// and a URL that describes it.
func NewBundleAtPath(path string) (charm.Bundle, *charm.URL, error) {
	if path == "" {
		return nil, nil, errgo.New("path to bundle not specified")
	}
	_, err := os.Stat(path)
	if isNotExistsError(err) {
		return nil, nil, os.ErrNotExist
	} else if err == nil && !isValidCharmOrBundlePath(path) {
		return nil, nil, InvalidPath(path)
	}
	b, err := charm.ReadBundle(path)
	if err != nil {
		if isNotExistsError(err) {
			return nil, nil, BundleNotFound(path)
		}
		return nil, nil, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	_, name := filepath.Split(absPath)
	url := &charm.URL{
		Schema:   "local",
		Name:     name,
		Series:   "bundle",
		Revision: 0,
	}
	return b, url, nil
}

// ReadBundleFile attempts to read the file at path
// and interpret it as a bundle.
func ReadBundleFile(path string) (*charm.BundleData, error) {
	f, err := os.Open(path)
	if err != nil {
		if isNotExistsError(err) {
			return nil, BundleNotFound(path)
		}
		return nil, err
	}
	defer f.Close()
	return charm.ReadBundleData(f)
}
