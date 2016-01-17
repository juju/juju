// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"path/filepath"
	"runtime"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type fops interface {
	// MkDir provides the functionality of gc.C.MkDir().
	MkDir() string
}

// RealPaths implements Paths for tests that do touch the filesystem.
type RealPaths struct {
	tools         string
	charm         string
	socket        string
	metricsspool  string
	componentDirs map[string]string
	fops          fops
}

func osDependentSockPath(c *gc.C) string {
	sockPath := filepath.Join(c.MkDir(), "test.sock")
	if runtime.GOOS == "windows" {
		return `\\.\pipe` + sockPath[2:]
	}
	return sockPath
}

func NewRealPaths(c *gc.C) RealPaths {
	return RealPaths{
		tools:         c.MkDir(),
		charm:         c.MkDir(),
		socket:        osDependentSockPath(c),
		metricsspool:  c.MkDir(),
		componentDirs: make(map[string]string),
		fops:          c,
	}
}

func (p RealPaths) GetMetricsSpoolDir() string {
	return p.metricsspool
}

func (p RealPaths) GetToolsDir() string {
	return p.tools
}

func (p RealPaths) GetCharmDir() string {
	return p.charm
}

func (p RealPaths) GetJujucSocket() string {
	return p.socket
}

func (p RealPaths) ComponentDir(name string) string {
	if dirname, ok := p.componentDirs[name]; ok {
		return dirname
	}
	p.componentDirs[name] = filepath.Join(p.fops.MkDir(), name)
	return p.componentDirs[name]
}

type StorageContextAccessor struct {
	CStorage map[names.StorageTag]*ContextStorage
}

func (s *StorageContextAccessor) StorageTags() ([]names.StorageTag, error) {
	tags := set.NewTags()
	for tag := range s.CStorage {
		tags.Add(tag)
	}
	storageTags := make([]names.StorageTag, len(tags))
	for i, tag := range tags.SortedValues() {
		storageTags[i] = tag.(names.StorageTag)
	}
	return storageTags, nil
}

func (s *StorageContextAccessor) Storage(tag names.StorageTag) (jujuc.ContextStorageAttachment, error) {
	storage, ok := s.CStorage[tag]
	if !ok {
		return nil, errors.NotFoundf("storage")
	}
	return storage, nil
}

type ContextStorage struct {
	CTag      names.StorageTag
	CKind     storage.StorageKind
	CLocation string
}

func (c *ContextStorage) Tag() names.StorageTag {
	return c.CTag
}

func (c *ContextStorage) Kind() storage.StorageKind {
	return c.CKind
}

func (c *ContextStorage) Location() string {
	return c.CLocation
}

type FakeTracker struct {
	leadership.Tracker
}

func (FakeTracker) ServiceName() string {
	return "service-name"
}
