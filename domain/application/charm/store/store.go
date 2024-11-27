// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"
	"encoding/base32"
	"fmt"
	"io"
	"os"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

const (
	// ErrNotFound is returned when the file is not found.
	ErrNotFound = errors.ConstError("file not found")
)

// CharmStore provides an API for storing and retrieving charm blobs.
type CharmStore struct {
	objectStoreGetter objectstore.ModelObjectStoreGetter
	encoder           *base32.Encoding
}

// NewCharmStore returns a new charm store instance.
func NewCharmStore(objectStoreGetter objectstore.ModelObjectStoreGetter) *CharmStore {
	return &CharmStore{
		objectStoreGetter: objectStoreGetter,
		encoder:           base32.StdEncoding.WithPadding(base32.NoPadding),
	}
}

// Store the charm at the specified path into the object store. It is expected
// that the archive already exists at the specified path. If the file isn't
// found, a [ErrNotFound] is returned.
func (s *CharmStore) Store(ctx context.Context, name string, path string, size int64, hash string) (objectstore.UUID, error) {
	objectStore, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return "", errors.Errorf("getting object store: %w", err)
	}

	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", errors.Errorf("%q: %w", path, ErrNotFound)
	} else if err != nil {
		return "", errors.Errorf("cannot open file %q: %w", path, err)
	}

	// Ensure that we close any open handles to the file.
	defer file.Close()

	// Generate a unique path for the file.
	unique, err := uuid.NewUUID()
	if err != nil {
		return "", errors.Errorf("cannot generate unique path")
	}

	// The name won't be unique and that's ok. In that case, we'll slap a
	// unique identifier at the end of the name. This can happen if you have
	// a charm with the same name but different content.
	uniqueName := fmt.Sprintf("%s-%s", name, s.encoder.EncodeToString(unique[:]))

	// Store the file in the object store.
	return objectStore.PutAndCheckHash(ctx, uniqueName, file, size, hash)
}

// Get retrieves a ReadCloser for the charm archive at the give path from
// the underlying storage.
// NOTE: It is up to the caller to verify the integrity of the data from the charm
// hash stored in DQLite.
func (s *CharmStore) Get(ctx context.Context, archivePath string) (io.ReadCloser, error) {
	store, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return nil, errors.Errorf("getting object store: %w", err)
	}
	reader, _, err := store.Get(ctx, archivePath)
	if err != nil {
		return nil, errors.Errorf("getting charm: %w", err)
	}
	return reader, nil
}
