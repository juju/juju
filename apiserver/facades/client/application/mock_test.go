// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"io"
	"sync"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jtesting "github.com/juju/testing"

	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/state"
	statestorage "github.com/juju/juju/state/storage"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

type mockStorage struct {
	state.StorageInstance
	jtesting.Stub
	tag   names.StorageTag
	owner names.Tag
}

func (a *mockStorage) Kind() state.StorageKind {
	return state.StorageKindFilesystem
}

func (a *mockStorage) StorageTag() names.StorageTag {
	return a.tag
}

func (a *mockStorage) Owner() (names.Tag, bool) {
	return a.owner, a.owner != nil
}

type blobs struct {
	sync.Mutex
	m map[string]bool // maps path to added (true), or deleted (false)
}

// Add adds a path to the list of known paths.
func (b *blobs) Add(path string) {
	b.Lock()
	defer b.Unlock()
	b.check()
	b.m[path] = true
}

// Remove marks a path as deleted, even if it was not previously Added.
func (b *blobs) Remove(path string) {
	b.Lock()
	defer b.Unlock()
	b.check()
	b.m[path] = false
}

func (b *blobs) check() {
	if b.m == nil {
		b.m = make(map[string]bool)
	}
}

type recordingStorage struct {
	statestorage.Storage
	putBarrier *sync.WaitGroup
	blobs      *blobs
}

func (s *recordingStorage) Put(path string, r io.Reader, size int64) error {
	if s.putBarrier != nil {
		// This goroutine has gotten to Put() so mark it Done() and
		// wait for the other goroutines to get to this point.
		s.putBarrier.Done()
		s.putBarrier.Wait()
	}
	if err := s.Storage.Put(path, r, size); err != nil {
		return errors.Trace(err)
	}
	s.blobs.Add(path)
	return nil
}

func (s *recordingStorage) Remove(path string) error {
	if err := s.Storage.Remove(path); err != nil {
		return errors.Trace(err)
	}
	s.blobs.Remove(path)
	return nil
}

type mockStoragePoolManager struct {
	jtesting.Stub
	poolmanager.PoolManager
	storageType storage.ProviderType
}

func (m *mockStoragePoolManager) Get(name string) (*storage.Config, error) {
	m.MethodCall(m, "Get", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return storage.NewConfig(name, m.storageType, map[string]interface{}{"foo": "bar"})
}

type mockStorageRegistry struct {
	jtesting.Stub
	storage.ProviderRegistry
}

type mockRepo struct {
	application.Repository
	*jtesting.CallMocker
	revisions map[string]int
}

func (m *mockRepo) DownloadCharm(resourceURL, _ string) (*charm.CharmArchive, error) {
	results := m.MethodCall(m, "DownloadCharm", resourceURL)
	if results == nil {
		return nil, errors.NotFoundf(`cannot retrieve %q: charm`, resourceURL)
	}
	return results[0].(*charm.CharmArchive), jtesting.TypeAssertError(results[1])
}
