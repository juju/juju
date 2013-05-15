// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"errors"
	"io/ioutil"
)

// FileVar represents a path to a file.
type FileVar struct {
	Path string
}

var ErrNoPath = errors.New("path not set")

// Set stores the chosen path name in f.Path.
func (f *FileVar) Set(v string) error {
	f.Path = v
	return nil
}

// Read returns the contents of the file.
func (f *FileVar) Read(ctx *Context) ([]byte, error) {
	if f.Path == "" {
		return nil, ErrNoPath
	}
	return ioutil.ReadFile(ctx.AbsPath(f.Path))
}

// String returns the path to the file.
func (f *FileVar) String() string {
	return f.Path
}
