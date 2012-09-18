package cmd

import (
	"errors"
	"io/ioutil"
)

// FileVar represents a path to a file. 
type FileVar struct {
	Path string
}

// Set opens the file. 
func (f *FileVar) Set(v string) error {
	f.Path = v
	return nil
}

// Read returns the contents of the file.
func (f *FileVar) Read(ctx *Context) ([]byte, error) {
	if f.Path == "" {
		return nil, errors.New("path not set")
	}
	return ioutil.ReadFile(f.Path)
}

// String returns the path to the file.
func (f *FileVar) String() string {
	return f.Path
}
