// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd

import (
	"errors"
	"io"
	"io/ioutil"
	"os"

	"github.com/juju/utils"
)

// FileVar represents a path to a file.
type FileVar struct {
	// Path is the path to the file.
	Path string

	// StdinMarkers are the Path values that should be interpreted as
	// stdin. If it is empty then stdin is not supported.
	StdinMarkers []string
}

var ErrNoPath = errors.New("path not set")

// Set stores the chosen path name in f.Path.
func (f *FileVar) Set(v string) error {
	f.Path = v
	return nil
}

// SetStdin sets StdinMarkers to the provided strings. If none are
// provided then the default of "-" is used.
func (f *FileVar) SetStdin(markers ...string) {
	if len(markers) == 0 {
		markers = append(markers, "-")
	}
	f.StdinMarkers = markers
}

// IsStdin determines whether or not the path represents stdin.
func (f FileVar) IsStdin() bool {
	for _, marker := range f.StdinMarkers {
		if f.Path == marker {
			return true
		}
	}
	return false
}

// Open opens the file.
func (f *FileVar) Open(ctx *Context) (io.ReadCloser, error) {
	if f.Path == "" {
		return nil, ErrNoPath
	}
	if f.IsStdin() {
		return ioutil.NopCloser(ctx.Stdin), nil
	}

	path, err := utils.NormalizePath(f.Path)
	if err != nil {
		return nil, err
	}
	return os.Open(ctx.AbsPath(path))
}

// Read returns the contents of the file.
func (f *FileVar) Read(ctx *Context) ([]byte, error) {
	if f.Path == "" {
		return nil, ErrNoPath
	}
	if f.IsStdin() {
		return ioutil.ReadAll(ctx.Stdin)
	}

	path, err := utils.NormalizePath(f.Path)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadFile(ctx.AbsPath(path))
}

// String returns the path to the file.
func (f *FileVar) String() string {
	return f.Path
}
