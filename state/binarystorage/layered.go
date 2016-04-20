// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package binarystorage

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

type layeredStorage []Storage

// NewLayeredStorage wraps multiple Storages such all of their metadata
// can be listed and fetched. The later entries in the list have lower
// precedence than the earlier ones. The first entry in the list is always
// used for mutating operations.
func NewLayeredStorage(s ...Storage) (Storage, error) {
	if len(s) <= 1 {
		return nil, errors.Errorf("expected multiple stores")
	}
	return layeredStorage(s), nil
}

// Add implements Storage.Open.
//
// This method operates on the first Storage passed to NewLayeredStorage.
func (s layeredStorage) Add(r io.Reader, m Metadata) error {
	return s[0].Add(r, m)
}

// Open implements Storage.Open.
//
// This method calls Open for each Storage passed to NewLayeredStorage in
// the order given, and returns the first result where the errors does not
// satisfy errors.IsNotFound.
func (s layeredStorage) Open(v string) (Metadata, io.ReadCloser, error) {
	var m Metadata
	var rc io.ReadCloser
	var err error
	for _, s := range s {
		m, rc, err = s.Open(v)
		if !errors.IsNotFound(err) {
			break
		}
	}
	return m, rc, err
}

// Metadata implements Storage.Metadata.
//
// This method calls Metadata for each Storage passed to NewLayeredStorage in
// the order given, and returns the first result where the errors does not
// satisfy errors.IsNotFound.
func (s layeredStorage) Metadata(v string) (Metadata, error) {
	var m Metadata
	var err error
	for _, s := range s {
		m, err = s.Metadata(v)
		if !errors.IsNotFound(err) {
			break
		}
	}
	return m, err
}

// AllMetadata implements Storage.AllMetadata.
//
// This method calls AllMetadata for each Storage passed to NewLayeredStorage
// in the order given, and accumulates the results. Any results from a Storage
// that have been returned from a Storage earlier in the list will be ignored.
func (s layeredStorage) AllMetadata() ([]Metadata, error) {
	seen := set.NewStrings()
	var all []Metadata
	for _, s := range s {
		sm, err := s.AllMetadata()
		if err != nil {
			return nil, err
		}
		for _, m := range sm {
			if seen.Contains(m.Version) {
				continue
			}
			all = append(all, m)
			seen.Add(m.Version)
		}
	}
	return all, nil
}
