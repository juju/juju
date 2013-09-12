// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filestorage

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/utils"
)

// fileStorageReader implements StorageReader backed
// by the local filesystem.
type fileStorageReader struct {
	path string
}

// fileStorage implements StorageReader and StorageWriter
// backed by the local filesystem.
type fileStorage struct {
	*fileStorageReader
}

// newFileStorageReader returns a new storage reader for
// a directory inside the local file system.
func NewFileStorageReader(path string) (environs.StorageReader, error) {
	p := filepath.Clean(path)
	fi, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsDir() {
		return nil, fmt.Errorf("specified source path is not a directory: %s", path)
	}
	return &fileStorageReader{p}, nil
}

func NewFileStorage(path string) (environs.Storage, error) {
	r, err := NewFileStorageReader(path)
	if err != nil {
		return nil, err
	}
	return &fileStorage{r.(*fileStorageReader)}, nil
}

// Get implements environs.StorageReader.Get.
func (f *fileStorageReader) Get(name string) (io.ReadCloser, error) {
	filename, err := f.URL(name)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// List implements environs.StorageReader.List.
func (f *fileStorageReader) List(prefix string) ([]string, error) {
	// Add one for the missing path separator.
	pathlen := len(f.path) + 1
	pattern := filepath.Join(f.path, prefix+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	list := []string{}
	for _, match := range matches {
		fi, err := os.Stat(match)
		if err != nil {
			return nil, err
		}
		if !fi.Mode().IsDir() {
			filename := match[pathlen:]
			list = append(list, filename)
		}
	}
	sort.Strings(list)
	return list, nil
}

// URL implements environs.StorageReader.URL.
func (f *fileStorageReader) URL(name string) (string, error) {
	return path.Join(f.path, name), nil
}

// ConsistencyStrategy implements environs.StorageReader.ConsistencyStrategy.
func (f *fileStorageReader) ConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

// Put implements environs.StorageWriter.Put.
func (f *fileStorage) Put(name string, r io.Reader, length int64) error {
	dir, _ := path.Split(name)
	if dir != "" {
		dir := filepath.Join(f.path, dir)
		if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
			return err
		}
	}
	// Write to a temporary file first, and then move (atomically).
	file, err := ioutil.TempFile("", "juju-filestorage-"+name)
	if err != nil {
		return err
	}
	_, err = io.CopyN(file, r, length)
	file.Close()
	if err != nil {
		os.Remove(file.Name())
		return err
	}
	filepath := filepath.Join(f.path, name)
	return os.Rename(file.Name(), filepath)
}

// Remove implements environs.StorageWriter.Remove.
func (f *fileStorage) Remove(name string) error {
	return os.RemoveAll(filepath.Join(f.path, name))
}

// RemoveAll implements environs.StorageWriter.RemoveAll.
func (f *fileStorage) RemoveAll() error {
	return os.RemoveAll(f.path)
}
