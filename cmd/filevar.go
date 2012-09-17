package cmd

import (
	"io"
	"os"
)

// FileVar represents a path to a file. If the flag is 
// valid FileVar.ReadCloser will be non nil and FileVar
// will be usable as an io.ReadCloser.
type FileVar struct {
	io.ReadCloser
	Path string
}

// Set opens the file. 
func (f *FileVar) Set(v string) error {
	file, err := os.Open(v)
	if err != nil {
		return err
	}
	f.ReadCloser = file
	f.Path = v
	return nil
}

// String returns the path to the file.
func (f *FileVar) String() string {
	return f.Path
}
