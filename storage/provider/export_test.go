// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"os"

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
	dirs set.Strings
}

func (m *mockDirFuncs) mkDirAll(path string, perm os.FileMode) error {
	m.dirs.Add(path)
	return nil
}

func (m *mockDirFuncs) lstat(name string) (fi os.FileInfo, err error) {
	if m.dirs.Contains(name) {
		return nil, nil
	}
	return nil, os.ErrNotExist
}

func RootfsFilesystemSource(storageDir string, run func(string, ...string) (string, error)) storage.FilesystemSource {
	return &rootfsFilesystemSource{
		&mockDirFuncs{set.NewStrings()},
		run,
		storageDir,
	}
}

// MountedDirs returns all the dirs which have been created during any CreateFilesystem calls
// on the specified filesystem source..
func MountedDirs(fsSource storage.FilesystemSource) set.Strings {
	return fsSource.(*rootfsFilesystemSource).dirFuncs.(*mockDirFuncs).dirs
}

func RootfsProvider(run func(string, ...string) (string, error)) storage.Provider {
	return &rootfsProvider{run}
}
