// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
	"strings"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/errors"
	objectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
	"github.com/juju/juju/internal/uuid"
)

const (
	// ErrNotFound is returned when the file is not found.
	ErrNotFound = errors.ConstError("file not found")

	// ErrFileToLarge is returned when the file is too large.
	ErrFileToLarge = errors.ConstError("file too large")

	// ErrCharmHashMismatch is returned when the charm hash does not match the expected hash.
	ErrCharmHashMismatch = errors.ConstError("charm hash mismatch")
)

// Digest contains the SHA256 and SHA384 hashes of a charm archive. This
// will be used to verify the integrity of the charm archive.
type Digest struct {
	SHA256 string
	SHA384 string
	Size   int64
}

// StoreResult contains the path of the stored charm archive, the unique name
// of the charm archive, and the object store UUID.
type StoreResult struct {
	ArchivePath     string
	UniqueName      string
	ObjectStoreUUID objectstore.UUID
}

// CharmStore provides an API for storing and retrieving charm blobs.
type CharmStore struct {
	objectStoreGetter objectstore.ModelObjectStoreGetter
	encoder           *base64.Encoding
}

// NewCharmStore returns a new charm store instance.
func NewCharmStore(objectStoreGetter objectstore.ModelObjectStoreGetter) *CharmStore {
	return &CharmStore{
		objectStoreGetter: objectStoreGetter,
		encoder:           base64.StdEncoding.WithPadding(base64.NoPadding),
	}
}

// Store the charm at the specified path into the object store. It is expected
// that the archive already exists at the specified path. If the file isn't
// found, a [ErrNotFound] is returned.
func (s *CharmStore) Store(ctx context.Context, path string, size int64, hash string) (StoreResult, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return StoreResult{}, errors.Errorf("%q: %w", path, ErrNotFound)
	} else if err != nil {
		return StoreResult{}, errors.Errorf("opening file %q: %w", path, err)
	}

	// Ensure that we close any open handles to the file.
	defer file.Close()

	// Generate a unique path for the file.
	unique, err := uuid.NewUUID()
	if err != nil {
		return StoreResult{}, errors.Errorf("cannot generate unique path")
	}
	uniqueName := s.encoder.EncodeToString(unique[:])

	// Store the file in the object store.
	objectStore, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return StoreResult{}, errors.Errorf("getting object store: %w", err)
	}

	uuid, err := objectStore.PutAndCheckHash(ctx, uniqueName, file, size, hash)
	if err != nil {
		return StoreResult{}, errors.Errorf("putting charm: %w", err)
	}
	return StoreResult{
		ArchivePath:     file.Name(),
		UniqueName:      uniqueName,
		ObjectStoreUUID: uuid,
	}, nil
}

// StoreFromReader stores the charm from the provided reader into the object
// store. The caller is expected to remove the temporary file after the call.
// IThis does not check the integrity of the charm hash.
func (s *CharmStore) StoreFromReader(ctx context.Context, reader io.Reader, hashPrefix string) (StoreResult, Digest, error) {
	file, err := os.CreateTemp("", "charm-")
	if err != nil {
		return StoreResult{}, Digest{}, errors.Errorf("creating temporary file: %w", err)
	}

	// Ensure that we close any open handles to the file.
	defer file.Close()

	// Store the file in the object store.
	objectStore, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return StoreResult{}, Digest{}, errors.Errorf("getting object store: %w", err)
	}

	// Generate a unique path for the file.
	unique, err := uuid.NewUUID()
	if err != nil {
		return StoreResult{}, Digest{}, errors.Errorf("cannot generate unique path")
	}
	uniqueName := s.encoder.EncodeToString(unique[:])

	// Copy the reader into the temporary file.
	sha256, sha384, size, err := storeAndComputeHashes(file, reader)
	if err != nil {
		return StoreResult{}, Digest{}, errors.Errorf("storing charm from reader: %w", err)
	}

	// Ensure that we sync the file to disk, as the process may crash before
	// the file is written to disk.
	if err := file.Sync(); err != nil {
		return StoreResult{}, Digest{}, errors.Errorf("syncing temporary file: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return StoreResult{}, Digest{}, errors.Errorf("seeking temporary file: %w", err)
	}

	if !strings.HasPrefix(sha256, hashPrefix) {
		return StoreResult{}, Digest{}, ErrCharmHashMismatch
	}

	uuid, err := objectStore.PutAndCheckHash(ctx, uniqueName, file, size, sha384)
	if err != nil {
		return StoreResult{}, Digest{}, errors.Errorf("putting charm: %w", err)
	}

	return StoreResult{
			ArchivePath:     file.Name(),
			UniqueName:      uniqueName,
			ObjectStoreUUID: uuid,
		}, Digest{
			SHA256: sha256,
			SHA384: sha384,
			Size:   size,
		}, nil
}

// Get retrieves a ReadCloser for the charm archive at the give path from
// the underlying storage.
// NOTE: It is up to the caller to verify the integrity of the data from the charm
// hash stored in DQLite.
func (s *CharmStore) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	store, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return nil, errors.Errorf("getting object store: %w", err)
	}
	reader, _, err := store.Get(ctx, path)
	if errors.Is(err, objectstoreerrors.ObjectNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, errors.Errorf("getting charm: %w", err)
	}
	return reader, nil
}

// GetBySHA256Prefix retrieves a ReadCloser for a charm archive who's SHA256 hash
// starts with the provided prefix.
func (s *CharmStore) GetBySHA256Prefix(ctx context.Context, sha256Prefix string) (io.ReadCloser, error) {
	store, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return nil, errors.Errorf("getting object store: %w", err)
	}
	reader, _, err := store.GetBySHA256Prefix(ctx, sha256Prefix)
	if errors.Is(err, objectstoreerrors.ObjectNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, errors.Errorf("getting charm: %w", err)
	}
	return reader, nil
}

func storeAndComputeHashes(writer io.Writer, reader io.Reader) (string, string, int64, error) {
	hasher256 := sha256.New()
	hasher384 := sha512.New384()

	size, err := io.Copy(writer, io.TeeReader(reader, io.MultiWriter(hasher256, hasher384)))
	if errors.Is(err, io.EOF) {
		return "", "", -1, ErrFileToLarge
	} else if err != nil {
		return "", "", -1, errors.Errorf("hashing charm: %w", err)
	}

	sha256 := hex.EncodeToString(hasher256.Sum(nil))
	sha384 := hex.EncodeToString(hasher384.Sum(nil))
	return sha256, sha384, size, nil
}
