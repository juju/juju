// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"io/ioutil"
	"os"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/file_mock.go github.com/juju/juju/cmd/k8sagent/utils FileReaderWriter

// FileReaderWriter provides methods for reading a file or writing to a file.
type FileReaderWriter interface {
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte, perm os.FileMode) error

	MkdirAll(path string, perm os.FileMode) error
	Symlink(oldname, newname string) error
}

// NewFileReaderWriter returns a new FileReaderWriter.
func NewFileReaderWriter() FileReaderWriter {
	return &fileReaderWriter{}
}

type fileReaderWriter struct{}

var _ FileReaderWriter = (*fileReaderWriter)(nil)

func (fileReaderWriter) ReadFile(filename string) ([]byte, error) {
	return ioutil.ReadFile(filename)
}

func (fileReaderWriter) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return ioutil.WriteFile(filename, data, perm)
}

func (fileReaderWriter) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (fileReaderWriter) Symlink(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}
