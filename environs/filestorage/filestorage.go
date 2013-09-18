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

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/utils"
)

// fileStorageReader implements StorageReader backed
// by the local filesystem.
type fileStorageReader struct {
	path string
}

// NewFileStorageReader returns a new storage reader for
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

func (f *fileStorageReader) fullPath(name string) string {
	return filepath.Join(f.path, name)
}

// Get implements environs.StorageReader.Get.
func (f *fileStorageReader) Get(name string) (io.ReadCloser, error) {
	filename := f.fullPath(name)
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
	return "file://" + filepath.Join(f.path, name), nil
}

// ConsistencyStrategy implements environs.StorageReader.ConsistencyStrategy.
func (f *fileStorageReader) ConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

type fileStorageWriter struct {
	fileStorageReader
	tmpdir string
}

// UseDefaultTmpDir may be passed into NewFileStorageWriter
// for the tmpdir argument, to signify that the default
// value should be used. See NewFileStorageWriter for more.
const UseDefaultTmpDir = ""

// NewFileStorageWriter returns a new read/write storag for
// a directory inside the local file system.
//
// A temporary directory may be specified, in which files will be written
// to before moving to the final destination. If specified, the temporary
// directory should be on the same filesystem as the storage directory
// to ensure atomicity. If tmpdir == UseDefaultTmpDir (""), then path+".tmp"
// will be used.
//
// If tmpdir == UseDefaultTmpDir, it will be created when Put is invoked,
// and will be removed afterwards. If tmpdir != UseDefaultTmpDir, it must
// already exist, and will never be removed.
func NewFileStorageWriter(path, tmpdir string) (environs.Storage, error) {
	reader, err := NewFileStorageReader(path)
	if err != nil {
		return nil, err
	}
	return &fileStorageWriter{*reader.(*fileStorageReader), tmpdir}, nil
}

func (f *fileStorageWriter) Put(name string, r io.Reader, length int64) error {
	fullpath := f.fullPath(name)
	dir := filepath.Dir(fullpath)
	tmpdir := f.tmpdir
	if tmpdir == UseDefaultTmpDir {
		tmpdir = f.path + ".tmp"
		if err := os.MkdirAll(tmpdir, 0755); err != nil && !os.IsExist(err) {
			return err
		}
		defer os.Remove(tmpdir)
	}
	if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
		return err
	}
	// Write to a temporary file first, and then move (atomically).
	file, err := ioutil.TempFile(tmpdir, "juju-filestorage-"+name)
	if err != nil {
		return err
	}
	_, err = io.CopyN(file, r, length)
	file.Close()
	if err != nil {
		os.Remove(file.Name())
		return err
	}
	return os.Rename(file.Name(), fullpath)
}

func (f *fileStorageWriter) Remove(name string) error {
	fullpath := f.fullPath(name)
	return os.Remove(fullpath)
}

func (f *fileStorageWriter) RemoveAll() error {
	return environs.RemoveAll(f)
}
