// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/core/lease"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/objectstore"
)

// MemoryMetadataService is an in-memory implementation of the objectstore
// MetadataService interface.
// This is purely for testing purposes, until we remove the dependency on
// state.
func MemoryMetadataService() objectstore.MetadataService {
	return &metadataService{
		store: newStore(),
	}
}

// MemoryObjectStore is an in-memory implementation of the objectstore
// interface.
// This is purely for testing purposes, until we remove the dependency on
// state.
func MemoryObjectStore() coreobjectstore.ObjectStoreMetadata {
	return &objectStore{
		store: newStore(),
	}
}

type metadataService struct {
	store *store
}

func (m *metadataService) ObjectStore() coreobjectstore.ObjectStoreMetadata {
	return &objectStore{
		store: m.store,
	}
}

type objectStore struct {
	store *store
}

// ListMetadata returns the persistence metadata for all paths.
func (s *objectStore) ListMetadata(ctx context.Context) ([]coreobjectstore.Metadata, error) {
	return s.store.list()
}

// GetMetadata implements objectstore.ObjectStoreMetadata.
func (s *objectStore) GetMetadata(ctx context.Context, path string) (coreobjectstore.Metadata, error) {
	if path == "" {
		return coreobjectstore.Metadata{}, errors.NotValidf("path cannot be empty")
	}

	return s.store.get(path)
}

// GetMetadataBySHA256 implements objectstore.ObjectStoreMetadata.
func (s *objectStore) GetMetadataBySHA256(ctx context.Context, sha256 string) (coreobjectstore.Metadata, error) {
	if sha256 == "" {
		return coreobjectstore.Metadata{}, errors.NotValidf("sha256 cannot be empty")
	}

	return s.store.getBySHA(sha256)
}

// GetMetadataBySHA256Prefix implements objectstore.ObjectStoreMetadata.
func (s *objectStore) GetMetadataBySHA256Prefix(ctx context.Context, sha256Prefix string) (coreobjectstore.Metadata, error) {
	if sha256Prefix == "" {
		return coreobjectstore.Metadata{}, errors.NotValidf("sha256 cannot be empty")
	}

	return s.store.getBySHAPrefix(sha256Prefix)
}

// PutMetadata implements objectstore.ObjectStoreMetadata.
func (s *objectStore) PutMetadata(ctx context.Context, metadata coreobjectstore.Metadata) (coreobjectstore.UUID, error) {
	return s.store.put(metadata)
}

// RemoveMetadata implements objectstore.ObjectStoreMetadata.
func (s *objectStore) RemoveMetadata(ctx context.Context, path string) error {
	if path == "" {
		return errors.NotValidf("path cannot be empty")
	}

	return s.store.remove(path)
}

// Watch implements objectstore.ObjectStoreMetadata.
func (*objectStore) Watch(context.Context) (watcher.Watcher[[]string], error) {
	return nil, errors.NotImplementedf("watching not implemented")
}

type uuidMetadata struct {
	uuid     coreobjectstore.UUID
	metadata coreobjectstore.Metadata
}

type store struct {
	mutex    sync.Mutex
	metadata map[string]uuidMetadata
}

func newStore() *store {
	return &store{
		metadata: make(map[string]uuidMetadata),
	}
}

func (s *store) list() ([]coreobjectstore.Metadata, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var metadata []coreobjectstore.Metadata
	for _, m := range s.metadata {
		metadata = append(metadata, m.metadata)
	}
	sort.Slice(metadata, func(i, j int) bool {
		return metadata[i].Path < metadata[j].Path
	})
	return metadata, nil
}

func (s *store) get(path string) (coreobjectstore.Metadata, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	m, ok := s.metadata[path]
	if !ok {
		return coreobjectstore.Metadata{}, errors.NotFoundf("metadata for %q", path)
	}
	return m.metadata, nil
}

func (s *store) getBySHA(sha256 string) (coreobjectstore.Metadata, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, m := range s.metadata {
		if m.metadata.SHA256 == sha256 {
			return m.metadata, nil
		}
	}
	return coreobjectstore.Metadata{}, errors.NotFoundf("metadata for SHA %q", sha256)
}

func (s *store) getBySHAPrefix(sha256Prefix string) (coreobjectstore.Metadata, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, m := range s.metadata {
		if strings.HasPrefix(m.metadata.SHA256, sha256Prefix) {
			return m.metadata, nil
		}
	}
	return coreobjectstore.Metadata{}, errors.NotFoundf("metadata for SHA %q", sha256Prefix)
}

func (s *store) put(metadata coreobjectstore.Metadata) (coreobjectstore.UUID, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	uuid, err := coreobjectstore.NewUUID()
	if err != nil {
		return "", errors.Annotate(err, "generating uuid")
	}

	s.metadata[metadata.Path] = uuidMetadata{
		uuid:     uuid,
		metadata: metadata,
	}
	return uuid, nil
}

func (s *store) remove(path string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.metadata, path)
	return nil
}

// MemoryClaimer is an in-memory implementation of the objectstore Claimer
// interface.
func MemoryClaimer() objectstore.Claimer {
	return &memoryClaimer{
		claims: make(map[string]*claim),
	}
}

type claim struct {
	expiry time.Time
	unique string
}

type memoryClaimer struct {
	mutex  sync.Mutex
	claims map[string]*claim
}

// Claim implements objectstore.Claimer.
func (m *memoryClaimer) Claim(ctx context.Context, hash string) (objectstore.ClaimExtender, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, ok := m.claims[hash]; ok {
		return nil, lease.ErrClaimDenied
	}

	now := time.Now()
	unqiue := utils.MustNewUUID().String()
	m.claims[hash] = &claim{
		expiry: now.Add(time.Minute),
		unique: unqiue,
	}
	return memoryExtender{fn: func() error {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		claim, ok := m.claims[hash]
		if !ok || claim.unique != unqiue {
			return lease.ErrNotHeld
		}

		m.claims[hash].expiry = time.Now().Add(time.Minute)

		return nil
	}, now: now}, nil
}

// Release implements objectstore.Claimer.
func (m *memoryClaimer) Release(ctx context.Context, hash string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.claims, hash)

	return nil
}

type memoryExtender struct {
	fn  func() error
	now time.Time
}

// Duration implements objectstore.ClaimExtender.
func (m memoryExtender) Duration() time.Duration {
	return time.Since(m.now)
}

// Extend implements objectstore.ClaimExtender.
func (m memoryExtender) Extend(ctx context.Context) error {
	return m.fn()
}
