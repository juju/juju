package jujutest

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
)

type VirtualFile struct {
	*bytes.Reader
}

var _ http.File = (*VirtualFile)(nil)

func (f *VirtualFile) Close() error {
	return nil
}

func (f *VirtualFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, nil
}

func (f *VirtualFile) Stat() (os.FileInfo, error) {
	return nil, fmt.Errorf("Can't stat VirtualFile")
}

type FileContent struct {
	Name    string
	Content string
}

type VirtualFileSystem struct {
	contents []FileContent
}

var _ http.FileSystem = (*VirtualFileSystem)(nil)

func (vfs *VirtualFileSystem) Open(name string) (http.File, error) {
	for _, fc := range vfs.contents {
		if fc.Name == name {
			reader := bytes.NewReader([]byte(fc.Content))
			return &VirtualFile{reader}, nil
		}
	}
	return nil, &os.PathError{Op: "Open", Path: name, Err: os.ErrNotExist}
}

func NewVFS(contents []FileContent) http.FileSystem {
	return &VirtualFileSystem{contents}
}
