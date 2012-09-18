package cmd

import (
	"io"
	"os"
	"errors"
	"fmt"
)

// FileVar represents a path to a file. 
type FileVar struct {
	Path *string
}

// Set opens the file. 
func (f *FileVar) Set(v string) error {
	f.Path = new(string)
	*f.Path = v
	return nil
}

// Open returns a io.ReadCloser to the file relative to the context.
func (f *FileVar) Open(ctx *Context) (io.ReadCloser, error) {
	if f.Path == nil {
		return nil, errors.New("path not set")
	}
	return os.Open(ctx.AbsPath(*f.Path))
}

// String returns the path to the file.
func (f *FileVar) String() string {
	return fmt.Sprint(f.Path)	
}
