// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"io"
	"os"

	"github.com/juju/errors"
)

// ReadSeekCloser is a minimal interface used for os.File.
type ReadSeekCloser interface {
	io.ReadCloser
	io.Seeker
}

// Filesystem is an interface providing access to the filesystem,
// either delegating to calling os functions or functions which
// always return an error.
type Filesystem interface {
	Stat(name string) (os.FileInfo, error)
	Open(name string) (ReadSeekCloser, error)
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	Create(name string) (*os.File, error)
	RemoveAll(path string) error
}

type osFilesystem struct{}

func (osFilesystem) Create(name string) (*os.File, error) {
	return os.Create(name)
}

func (osFilesystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (osFilesystem) Open(name string) (ReadSeekCloser, error) {
	return os.Open(name)
}

func (osFilesystem) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

func (osFilesystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

var notSupported = errors.NotSupportedf("access to filesystem")

type restrictedFilesystem struct{}

func (restrictedFilesystem) Create(string) (*os.File, error) {
	return nil, notSupported
}

func (restrictedFilesystem) RemoveAll(string) error {
	return notSupported
}

func (restrictedFilesystem) Open(string) (ReadSeekCloser, error) {
	return nil, notSupported
}

func (restrictedFilesystem) OpenFile(string, int, os.FileMode) (*os.File, error) {
	return nil, notSupported
}

func (restrictedFilesystem) Stat(string) (os.FileInfo, error) {
	return nil, notSupported
}

// SetFilesystem sets the Filesystem instance on the command.
func (c *CommandBase) SetFilesystem(fs Filesystem) {
	c.filesystem = fs
}

// Filesystem returns an instance that provides access to
// the filesystem, either delegating to calling os functions
// or functions which always return an error.
func (c *CommandBase) Filesystem() Filesystem {
	if c.filesystem == nil {
		c.filesystem = osFilesystem{}
	}
	return c.filesystem
}
