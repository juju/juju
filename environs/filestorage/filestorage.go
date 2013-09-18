// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filestorage

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/utils"
)

// fileStorageReader implements StorageReader backed
// by the local filesystem.
type fileStorageReader struct {
	path string
}

// newFileStorageReader returns a new storage reader for
// a directory inside the local file system.
func NewFileStorageReader(path string) (storage.StorageReader, error) {
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

func (f *fileStorageReader) fullPath(name string) string {
	return filepath.Join(f.path, name)
}

// Get implements storage.StorageReader.Get.
func (f *fileStorageReader) Get(name string) (io.ReadCloser, error) {
	filename := f.fullPath(name)
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// List implements storage.StorageReader.List.
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

// URL implements storage.StorageReader.URL.
func (f *fileStorageReader) URL(name string) (string, error) {
	return "file://" + filepath.Join(f.path, name), nil
}

// ConsistencyStrategy implements storage.StorageReader.ConsistencyStrategy.
func (f *fileStorageReader) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

// ShouldRetry is specified in the StorageReader interface.
func (f *fileStorageReader) ShouldRetry(err error) bool {
	return false
}

type fileStorageWriter struct {
	fileStorageReader
}

func NewFileStorageWriter(path string) (storage.Storage, error) {
	reader, err := NewFileStorageReader(path)
	if err != nil {
		return nil, err
	}
	return &fileStorageWriter{*reader.(*fileStorageReader)}, nil
}

func (f *fileStorageWriter) Put(name string, r io.Reader, length int64) error {
	fullpath := f.fullPath(name)
	dir := filepath.Dir(fullpath)
	if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
		return err
	}
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(fullpath, data, 0644)
}

func (f *fileStorageWriter) Remove(name string) error {
	fullpath := f.fullPath(name)
	return os.Remove(fullpath)
}

func (f *fileStorageWriter) RemoveAll() error {
	return storage.RemoveAll(f)
}
