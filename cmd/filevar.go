package cmd

import (
	"os"
	"io"

        "launchpad.net/gnuflag"
)

// FileVar represents a path to a file
type FileVar struct {
	io.Reader
	Name string
}

// Set opens the file. 
func (f *FileVar) Set(v string) error {
	file, err := os.Open(v)
	if err != nil { return err }
	f.Reader = file
	f.Name = v
	return nil
}

// String returns the name of the file.
func (f *FileVar) String() string {
	return f.Name
}

func (f *FileVar) AddFlags(fs *gnuflag.FlagSet, name, desc string) {
        fs.Var(f, name, desc)
}

