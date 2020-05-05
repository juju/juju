// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"os"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/names/v4"

	"github.com/juju/juju/storage"
)

var Getpagesize = &getpagesize

func LoopVolumeSource(
	etcDir string,
	storageDir string,
	run func(string, ...string) (string, error),
) (storage.VolumeSource, *MockDirFuncs) {
	dirFuncs := &MockDirFuncs{
		osDirFuncs{run},
		etcDir,
		set.NewStrings(),
	}
	return &loopVolumeSource{dirFuncs, run, storageDir}, dirFuncs
}

func LoopProvider(
	run func(string, ...string) (string, error),
) storage.Provider {
	return &loopProvider{run}
}

func NewMockManagedFilesystemSource(
	etcDir string,
	run func(string, ...string) (string, error),
	volumeBlockDevices map[names.VolumeTag]storage.BlockDevice,
	filesystems map[names.FilesystemTag]storage.Filesystem,
) (storage.FilesystemSource, *MockDirFuncs) {
	dirFuncs := &MockDirFuncs{
		osDirFuncs{run},
		etcDir,
		set.NewStrings(),
	}
	return &managedFilesystemSource{
		run, dirFuncs,
		volumeBlockDevices, filesystems,
	}, dirFuncs
}

var _ dirFuncs = (*MockDirFuncs)(nil)

// MockDirFuncs stub out the real mkdir and lstat functions from stdlib.
type MockDirFuncs struct {
	osDirFuncs
	fakeEtcDir string
	Dirs       set.Strings
}

func (m *MockDirFuncs) etcDir() string {
	return m.fakeEtcDir
}

func (m *MockDirFuncs) mkDirAll(path string, perm os.FileMode) error {
	m.Dirs.Add(path)
	return nil
}

type MockFileInfo struct {
	isDir bool
}

func (m *MockFileInfo) IsDir() bool {
	return m.isDir
}

func (m *MockFileInfo) Name() string {
	return ""
}

func (m *MockFileInfo) Size() int64 {
	return 0
}

func (m *MockFileInfo) Mode() os.FileMode {
	return 0
}

func (m *MockFileInfo) ModTime() time.Time {
	return time.Now()
}
func (m *MockFileInfo) Sys() interface{} {
	return nil
}

func (m *MockDirFuncs) lstat(name string) (os.FileInfo, error) {
	if name == "file" || name == "exists" || m.Dirs.Contains(name) {
		return &MockFileInfo{name != "file"}, nil
	}
	return nil, os.ErrNotExist
}

func (m *MockDirFuncs) fileCount(name string) (int, error) {
	if strings.HasSuffix(name, "/666") {
		return 2, nil
	}
	return 0, nil
}

func RootfsFilesystemSource(etcDir, storageDir string, run func(string, ...string) (string, error)) (storage.FilesystemSource, *MockDirFuncs) {
	d := &MockDirFuncs{
		osDirFuncs{run},
		etcDir,
		set.NewStrings(),
	}
	return &rootfsFilesystemSource{d, run, storageDir}, d
}

func RootfsProvider(run func(string, ...string) (string, error)) storage.Provider {
	return &rootfsProvider{run}
}

func TmpfsFilesystemSource(etcDir, storageDir string, run func(string, ...string) (string, error)) storage.FilesystemSource {
	return &tmpfsFilesystemSource{
		&MockDirFuncs{
			osDirFuncs{run},
			etcDir,
			set.NewStrings(),
		},
		run,
		storageDir,
	}
}

func TmpfsProvider(run func(string, ...string) (string, error)) storage.Provider {
	return &tmpfsProvider{run}
}
