// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"os"
	"time"

	"github.com/juju/utils/set"

	"github.com/juju/juju/storage"
)

func LoopVolumeSource(storageDir string, run func(string, ...string) (string, error)) storage.VolumeSource {
	return &loopVolumeSource{run, storageDir}
}

func LoopProvider(run func(string, ...string) (string, error)) storage.Provider {
	return &loopProvider{run}
}

var _ dirFuncs = (*mockDirFuncs)(nil)

// mockDirFuncs stub out the real mkdir and lstat functions from stdlib.
type mockDirFuncs struct {
	osDirFuncs
	dirs set.Strings
}

func (m *mockDirFuncs) mkDirAll(path string, perm os.FileMode) error {
	m.dirs.Add(path)
	return nil
}

type mockFileInfo struct {
	isDir bool
}

func (m *mockFileInfo) IsDir() bool {
	return m.isDir
}

func (m *mockFileInfo) Name() string {
	return ""
}

func (m *mockFileInfo) Size() int64 {
	return 0
}

func (m *mockFileInfo) Mode() os.FileMode {
	return 0
}

func (m *mockFileInfo) ModTime() time.Time {
	return time.Now()
}
func (m *mockFileInfo) Sys() interface{} {
	return nil
}

func (m *mockDirFuncs) lstat(name string) (os.FileInfo, error) {
	if name == "file" || m.dirs.Contains(name) {
		return &mockFileInfo{name != "file"}, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockDirFuncs) fileCount(name string) (int, error) {
	if name == "/mnt/notempty" {
		return 2, nil
	}
	return 0, nil
}

func RootfsFilesystemSource(storageDir string, run func(string, ...string) (string, error)) storage.FilesystemSource {
	return &rootfsFilesystemSource{
		&mockDirFuncs{
			osDirFuncs{run},
			set.NewStrings(),
		},
		run,
		storageDir,
	}
}

func RootfsProvider(run func(string, ...string) (string, error)) storage.Provider {
	return &rootfsProvider{run}
}

func TmpfsFilesystemSource(storageDir string, run func(string, ...string) (string, error)) storage.FilesystemSource {
	return &tmpfsFilesystemSource{
		&mockDirFuncs{
			osDirFuncs{run},
			set.NewStrings(),
		},
		run,
		storageDir,
	}
}

func TmpfsProvider(run func(string, ...string) (string, error)) storage.Provider {
	return &tmpfsProvider{run}
}

// MountedDirs returns all the dirs which have been created during any CreateFilesystem calls
// on the specified filesystem source..
func MountedDirs(fsSource storage.FilesystemSource) set.Strings {
	switch fs := fsSource.(type) {
	case *rootfsFilesystemSource:
		return fs.dirFuncs.(*mockDirFuncs).dirs
	case *tmpfsFilesystemSource:
		return fs.dirFuncs.(*mockDirFuncs).dirs
	}
	panic("unexpectd type")
}
