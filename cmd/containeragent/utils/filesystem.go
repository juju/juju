// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"io"
	"os"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/file_mock.go github.com/juju/juju/cmd/containeragent/utils FileReaderWriter

// FileReaderWriter provides methods for reading a file or writing to a file.
type FileReaderWriter interface {
	Stat(filename string) (os.FileInfo, error)
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte, perm os.FileMode) error

	Reader(filename string) (io.ReadCloser, error)
	Writer(filename string, perm os.FileMode) (io.WriteCloser, error)

	MkdirAll(path string, perm os.FileMode) error
	Symlink(oldname, newname string) error
	RemoveAll(path string) error
}

// NewFileReaderWriter returns a new FileReaderWriter.
func NewFileReaderWriter() FileReaderWriter {
	return &fileReaderWriter{}
}

type fileReaderWriter struct{}

var _ FileReaderWriter = (*fileReaderWriter)(nil)

func (fileReaderWriter) Stat(filename string) (os.FileInfo, error) {
	return os.Stat(filename)
}

func (fileReaderWriter) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func (fileReaderWriter) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func (fileReaderWriter) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (fileReaderWriter) Symlink(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}

func (fileReaderWriter) Reader(filename string) (io.ReadCloser, error) {
	return os.OpenFile(filename, os.O_RDONLY, 0)
}

func (fileReaderWriter) Writer(filename string, perm os.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
}

func (fileReaderWriter) RemoveAll(path string) error {
	return os.RemoveAll(path)
}
