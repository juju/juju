// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugin

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
)

// FileOps exposes all the filesystem operations needed by plugins.
type FileOps interface {
	// MkdirAll provides the functionality of os.MkdirAll.
	MkdirAll(path string, perm os.FileMode) error
	// ReadFile provides the functionality of ioutil.ReadFile.
	ReadFile(filename string) ([]byte, error)
	// WriteFile provides the functionality of ioutil.WriteFile.
	WriteFile(filename string, data []byte, perm os.FileMode) error
}

// Paths provides the paths related to a plugin.
type Paths struct {
	// Plugin is the name of the plugin.
	Plugin string
	// DataDir is the path to the plugin's data dir.
	DataDir string
	// ExecutablePathFile is the location of the file where the path
	// to the plugin's executable is stored.
	ExecutablePathFile string

	// Fops provides the filesystem operations used by Paths.
	Fops FileOps
}

// NewPaths returns a new Paths for the named plugin, relative to the
// given data dir.
func NewPaths(baseDataDir, name string) Paths {
	p := Paths{
		Plugin:  name,
		DataDir: filepath.Join(baseDataDir, "plugins", name),
		Fops:    &fileOps{},
	}
	p.ExecutablePathFile = p.resolve(".executable")
	return p
}

// resolve returns the path composed of the provided parts, relative
// to the plugin's data dir.
func (p Paths) resolve(parts ...string) string {
	return filepath.Join(append([]string{p.DataDir}, parts...)...)
}

// Init initializes the plugin's data directory.
func (p Paths) Init(executable string) error {
	if err := p.Fops.MkdirAll(p.DataDir, 0755); err != nil {
		return errors.Trace(err)
	}

	if err := p.Fops.WriteFile(p.ExecutablePathFile, []byte(executable), 0644); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// Executable returns the path to the plugin's executable file.
func (p Paths) Executable() (string, error) {
	data, err := p.Fops.ReadFile(p.ExecutablePathFile)
	if os.IsNotExist(err) {
		return "", errors.NotFoundf(p.ExecutablePathFile)
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(data), nil
}

// TODO(ericsnow) fileOps should move to the utils repo.

type fileOps struct{}

// MkdirAll implements FileOps.
func (fileOps) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// ReadFile implements FileOps.
func (fileOps) ReadFile(filename string) ([]byte, error) {
	return ioutil.ReadFile(filename)
}

// WriteFile implements FileOps.
func (fileOps) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return ioutil.WriteFile(filename, data, perm)
}
